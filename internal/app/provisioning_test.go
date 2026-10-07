package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type routeVerifier struct{}

func (routeVerifier) Verify(context.Context, string) (string, error) { return "verified-uid", nil }

type routeAccounts struct{ role string }

func (a routeAccounts) FindByFirebaseUID(context.Context, string) (domain.Account, error) {
	return domain.Account{FirebaseUID: "verified-uid", Role: a.role, IsActive: true}, nil
}

func TestProvisioningRouteRequiresAdmin(t *testing.T) {
	for _, role := range []string{domain.RoleAdmin, domain.RoleWarden, domain.RoleSubWarden, domain.RoleSecurityStaff, domain.RoleStudent} {
		t.Run(role, func(t *testing.T) {
			called := false
			authenticate := authentication.Middleware(routeVerifier{}, routeAccounts{role}, time.Second)
			handler := newAPIHandler(func(context.Context) error { return nil }, time.Second, authenticate, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }))
			r := httptest.NewRequest("POST", "/api/v1/users", nil)
			r.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			want := 403
			if role == domain.RoleAdmin {
				want = 204
			}
			if w.Code != want || called != (role == domain.RoleAdmin) {
				t.Fatalf("status=%d called=%t", w.Code, called)
			}
			for _, tc := range []struct {
				method, path string
				want         int
			}{{"POST", "/api/v1/users", 401}, {"GET", "/api/v1/users", 405}, {"GET", "/health", 200}, {"GET", "/ready", 200}} {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
				if w.Code != tc.want {
					t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
				}
			}
		})
	}
}
