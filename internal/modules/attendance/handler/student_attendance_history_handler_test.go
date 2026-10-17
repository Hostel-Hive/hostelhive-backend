package handler

import (
	"context"
	"encoding/json"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type historyLookup struct {
	role   string
	active bool
}

func (a historyLookup) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return identity.Account{FirebaseUID: "admin", Role: a.role, IsActive: a.active}, nil
}

type historyRepo struct {
	calls int
	err   error
}

func (f *historyRepo) OwnEvents(ctx context.Context, actor string, q dto.StudentHistoryFilter) (d.StudentHistoryPage, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing handler timeout")
	}
	if actor != "admin" {
		panic("unverified identity")
	}
	f.calls++
	return d.StudentHistoryPage{Items: []d.StudentMovement{}, Limit: q.Limit, Offset: q.Offset, Timezone: q.Timezone}, f.err
}
func TestStudentHistoryQueryShape(t *testing.T) {
	for _, tc := range []struct {
		query, body string
		valid       bool
	}{
		{"", "", true}, {"?from=2026-10-01&to=2026-10-02&direction=IN&limit=2&offset=1", "", true},
		{"?unknown=x", "", false}, {"?student_id=x", "", false}, {"?user_id=x", "", false},
		{"?limit=1&limit=2", "", false}, {"?direction=IN&direction=OUT", "", false}, {"?from=2026-10-01&from=2026-10-02", "", false},
		{"?direction=", "", false}, {"?from=", "", false}, {"?offset=", "", false}, {"?to=", "", false}, {"?limit=x", "", false}, {"?offset=x", "", false},
		{"?limit=9999999999999999999999999", "", false}, {"?from=%ZZ", "", false}, {"?x=1;y=2", "", false},
		{"", "{}", false}, {"", strings.Repeat("a", 100), false},
	} {
		w := httptest.NewRecorder()
		q, ok := studentHistoryQuery(w, httptest.NewRequest("GET", "/"+tc.query, strings.NewReader(tc.body)))
		if ok != tc.valid {
			t.Fatalf("query %q body %q: %v", tc.query, tc.body, ok)
		}
		if ok && tc.query == "" && (q.Limit != 20 || q.Offset != 0) {
			t.Fatal("defaults")
		}
	}
}
func TestStudentHistoryHTTPAccess(t *testing.T) {
	for _, tc := range []struct {
		role          string
		active, token bool
		err           error
		status        int
	}{
		{"student", true, true, nil, 200}, {"student", true, false, nil, 401}, {"student", false, true, nil, 403},
		{"admin", true, true, nil, 403}, {"warden", true, true, nil, 403}, {"sub_warden", true, true, nil, 403}, {"security_staff", true, true, nil, 403},
		{"student", true, true, d.ErrNotFound, 404}, {"student", true, true, d.ErrForbidden, 403}, {"student", true, true, errors.New("private SQL"), 503},
	} {
		f := &historyRepo{err: tc.err}
		api := NewStudentHistoryAPI(svc.NewStudentHistory(f, time.UTC), time.Second)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/me/attendance/events", nil)
		if tc.token {
			r.Header.Set("Authorization", "Bearer token")
		}
		m.Middleware(dashboardVerifier{}, historyLookup{tc.role, tc.active}, time.Second)(http.HandlerFunc(api.Events)).ServeHTTP(w, r)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "private SQL") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
		if tc.status == 200 {
			var p d.StudentHistoryPage
			if json.Unmarshal(w.Body.Bytes(), &p) != nil || p.Items == nil || p.Total != 0 || p.Limit != 20 {
				t.Fatal("empty page")
			}
		}
		if tc.status == 401 || (!tc.active || tc.role != "student") {
			if f.calls != 0 {
				t.Fatal("unauthorized repository call")
			}
		}
	}
	api := NewStudentHistoryAPI(nil, time.Second)
	w := httptest.NewRecorder()
	api.Events(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 401 {
		t.Fatal("direct unauthenticated handler")
	}
	// Semantic filter validation must fail before querying persistence.
	for _, query := range []string{"?limit=0", "?offset=-1", "?direction=in", "?from=invalid&to=invalid"} {
		f := &historyRepo{}
		api := NewStudentHistoryAPI(svc.NewStudentHistory(f, time.UTC), time.Second)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/"+query, nil)
		r.Header.Set("Authorization", "Bearer token")
		m.Middleware(dashboardVerifier{}, historyLookup{"student", true}, time.Second)(http.HandlerFunc(api.Events)).ServeHTTP(w, r)
		if w.Code != 400 || f.calls != 0 {
			t.Fatal("invalid filter reached database")
		}
	}
}
