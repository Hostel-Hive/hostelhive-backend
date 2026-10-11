package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type unavailableDashboard struct{}

func (unavailableDashboard) PresenceStudents(context.Context, string, dto.PresenceFilter) (d.PresencePage, error) {
	return d.PresencePage{}, d.ErrUnavailable
}
func (unavailableDashboard) PresenceSummary(context.Context, string, string) (d.PresenceSummary, error) {
	return d.PresenceSummary{}, d.ErrUnavailable
}

func TestDashboardQueryShape(t *testing.T) {
	for _, tc := range []struct {
		query, body    string
		listing, valid bool
	}{
		{"", "", true, true}, {"?q=Ravi&status=IN&limit=10&offset=2", "", true, true},
		{"?q=Ravi", "", false, true}, {"?q=x&q=y", "", true, false}, {"?q=%ZZ", "", true, false},
		{"?limit=x", "", true, false}, {"?status=", "", true, false}, {"?status=OUT", "", false, false},
		{"?unknown=x", "", true, false}, {"", "{}", true, false},
	} {
		r := httptest.NewRequest("GET", "/"+tc.query, strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		filter, ok := dashboardQuery(w, r, tc.listing)
		if ok != tc.valid {
			t.Fatalf("query %s accepted=%v", tc.query, ok)
		}
		if tc.query == "" && tc.valid && filter.Limit != 20 {
			t.Fatal("default limit")
		}
	}
}

func TestDashboardRequiresAuthorityAndMapsFailure(t *testing.T) {
	api := NewDashboardAPI(unavailableDashboard{}, time.Second)
	for _, endpoint := range []func(*httptest.ResponseRecorder){
		func(w *httptest.ResponseRecorder) { api.Students(w, httptest.NewRequest("GET", "/", nil)) },
		func(w *httptest.ResponseRecorder) { api.Summary(w, httptest.NewRequest("GET", "/", nil)) },
	} {
		w := httptest.NewRecorder()
		endpoint(w)
		if w.Code != 401 {
			t.Fatal("missing account accepted")
		}
	}
	for _, call := range []func(*httptest.ResponseRecorder){
		func(w *httptest.ResponseRecorder) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", "Bearer token")
			m.Middleware(dashboardVerifier{}, dashboardAccountLookup{}, time.Second)(http.HandlerFunc(api.Students)).ServeHTTP(w, r)
		},
		func(w *httptest.ResponseRecorder) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", "Bearer token")
			m.Middleware(dashboardVerifier{}, dashboardAccountLookup{}, time.Second)(http.HandlerFunc(api.Summary)).ServeHTTP(w, r)
		},
	} {
		w := httptest.NewRecorder()
		call(w)
		if w.Code != 503 {
			t.Fatal("database failure did not map to 503")
		}
	}
}

type dashboardVerifier struct{}

func (dashboardVerifier) Verify(context.Context, string) (string, error) { return "admin", nil }

type dashboardAccountLookup struct{}

func (dashboardAccountLookup) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return identity.Account{FirebaseUID: "admin", Role: identity.RoleAdmin, IsActive: true}, nil
}
