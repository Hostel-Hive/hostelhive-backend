package server

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/accountmanagement"
	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A nil repository would panic if any unauthorized request reached management.
func TestAccountManagementRoutesProtected(t *testing.T) {
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		authenticate := authentication.Middleware(routeVerifier{}, routeAccounts{role}, time.Second)
		h := newAPIHandler(func(context.Context) error { return nil }, time.Second, authenticate, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), accountmanagement.NewAPI(nil, nil, time.Second))
		for _, tc := range []struct{ method, path, body string }{{"GET", "/api/v1/users?limit=0", ""}, {"PATCH", "/api/v1/users/not-uuid/role", `{"role":"admin"}`}, {"POST", "/api/v1/users/not-uuid/deactivate", ""}} {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer test")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			want := 403
			if role == "admin" {
				want = 400
			}
			if w.Code != want {
				t.Fatalf("%s %s %d", role, tc.path, w.Code)
			}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if w.Code != 401 {
				t.Fatal("missing token accepted")
			}
		}
	}
}
