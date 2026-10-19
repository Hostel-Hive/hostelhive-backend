package middleware_test

import (
	"context"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStudentSetupCannotBypassProtectedRoutes(t *testing.T) {
	verifier := verifyFunc(func(context.Context, string) (string, error) { return "uid", nil })
	for _, pending := range []bool{true, false} {
		for _, tc := range []struct {
			method, path string
			allowed      bool
		}{
			{"GET", "/api/v1/me", true}, {"GET", "/api/v1/me?role=admin", true}, {"POST", "/api/v1/me/setup/complete", true},
			{"GET", "/api/v1/me/student-profile", false}, {"GET", "/api/v1/me/qr", false}, {"GET", "/api/v1/me/attendance/events", false},
			{"POST", "/api/v1/complaints", false}, {"POST", "/api/v1/leaves", false}, {"GET", "/api/v1/notifications", false},
			{"PUT", "/api/v1/me/push-devices/id", false}, {"POST", "/api/v1/attendance/realtime/session", false},
			{"GET", "/api/v1/me/setup/complete", false}, {"POST", "/api/v1/me", false},
		} {
			called := false
			account := accountFunc(func(context.Context, string) (d.Account, error) {
				return d.Account{FirebaseUID: "uid", Role: "student", IsActive: true, SetupRequired: pending}, nil
			})
			h := m.Middleware(verifier, account, time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }))
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			expected := !pending || tc.allowed
			if called != expected {
				t.Fatal(pending, tc, w.Code)
			}
			if !expected && w.Code != 403 {
				t.Fatal("setup bypass")
			}
		}
	}
}
