package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type historyStore struct {
	calls    int
	actor    string
	filter   domain.PassFilter
	deadline bool
	err      error
}

func (f *historyStore) OwnPasses(ctx context.Context, actor string, filter domain.PassFilter) (domain.PassPage, error) {
	f.calls++
	f.actor, f.filter = actor, filter
	_, f.deadline = ctx.Deadline()
	return domain.PassPage{Items: []domain.PassItem{}, Limit: filter.Limit, Offset: filter.Offset}, f.err
}

func TestPassHistoryHTTP(t *testing.T) {
	f := &historyStore{}
	account := identity.Account{FirebaseUID: "student", Role: "student", IsActive: true}
	invoke := func(method, query, body, token string) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		leave.RegisterPassHistory(handler.NewPassHistory(service.NewPassHistory(f), time.Second), mux, middleware.Middleware(goingHomeVerifier{}, goingHomeAccounts{account}, time.Second))
		r := httptest.NewRequest(method, "/api/v1/me/passes"+query, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	w := invoke("GET", "", "", "student")
	var page domain.PassPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Items == nil || page.Limit != 20 || page.Total != 0 || w.Header().Get("Cache-Control") != "no-store" || f.actor != "student" || !f.deadline {
		t.Fatal(w.Code, w.Body, f)
	}
	w = invoke("GET", "?type=late_access&status=cancelled&limit=1&offset=2", "", "student")
	if w.Code != 200 || f.filter != (domain.PassFilter{Type: "late_access", Status: "cancelled", Limit: 1, Offset: 2}) {
		t.Fatal(w.Code, w.Body, f)
	}
	before := f.calls
	for _, query := range []string{
		"?student_id=other", "?user_id=other", "?unknown=1", "?limit=", "?limit=0", "?limit=101", "?offset=-1", "?offset=1000001",
		"?limit=1&limit=2", "?offset=0&offset=1", "?limit=99999999999999999999999", "?limit=1.5", "?limit=%zz", "?limit=1;foo=1",
		"?type=going_home", "?type=LEAVE", "?type=", "?type=leave&type=late_access", "?status=consumed", "?status=", "?status=pending&status=approved",
	} {
		if w := invoke("GET", query, "", "student"); w.Code != 400 {
			t.Fatal(query, w.Code, w.Body)
		}
	}
	for _, body := range []string{"{}", "null", " "} {
		if w := invoke("GET", "", body, "student"); w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if w := invoke(method, "", "", "student"); w.Code != 405 {
			t.Fatal(method, w.Code)
		}
	}
	for _, token := range []string{"", "bad"} {
		if w := invoke("GET", "", "", token); w.Code != 401 {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff"} {
		account.Role = role
		if w := invoke("GET", "", "", "student"); w.Code != 403 {
			t.Fatal(role, w.Code, w.Body)
		}
	}
	account.Role, account.IsActive = "student", false
	if w := invoke("GET", "", "", "student"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	account.IsActive, account.SetupRequired = true, true
	if w := invoke("GET", "", "", "student"); w.Code != 403 || !strings.Contains(w.Body.String(), "setup_required") {
		t.Fatal(w.Code, w.Body)
	}
	if f.calls != before {
		t.Fatal("invalid or unauthorized input reached repository")
	}
	account.SetupRequired = false
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalid, 400, "invalid_input"}, {domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrIneligible, 409, "active_student_required"}, {errors.New("private database details"), 503, "leave_unavailable"},
	} {
		f.err = tc.err
		w := invoke("GET", "", "", "student")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "private database details") {
			t.Fatal(w.Code, w.Body)
		}
	}
}
