package app

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrowserPreflightAndAuthenticatedRoutes(t *testing.T) {
	const origin = "https://web-kohl-delta-90.vercel.app"
	var calls atomic.Int64
	authenticate := authentication.Middleware(routeVerifier{}, routeAccounts{domain.RoleStudent}, time.Second)
	protected := func(next http.Handler) http.Handler {
		wrapped := authenticate(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); wrapped.ServeHTTP(w, r) })
	}
	mux := newAPIHandler(func(context.Context) error { return nil }, time.Second, protected, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("Student reached admin operation") }))
	server := httptest.NewServer(authentication.CORS([]string{origin})(mux))
	defer server.Close()
	for _, tc := range []struct {
		method, path, preflight string
		token                   bool
		want, calls             int
	}{
		{"OPTIONS", "/api/v1/me", "GET", false, 204, 0},
		{"GET", "/api/v1/me", "", false, 401, 1},
		{"GET", "/api/v1/me", "", true, 200, 2},
		{"OPTIONS", "/api/v1/users", "POST", false, 204, 2},
		{"POST", "/api/v1/users", "", true, 403, 3},
		{"GET", "/api/v1/missing", "", false, 404, 3},
		{"POST", "/api/v1/me", "", true, 405, 3},
		{"GET", "/health", "", false, 200, 3},
		{"GET", "/ready", "", false, 200, 3},
	} {
		req, err := http.NewRequest(tc.method, server.URL+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", origin)
		if tc.preflight != "" {
			req.Header.Set("Access-Control-Request-Method", tc.preflight)
			req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		}
		if tc.token {
			req.Header.Set("Authorization", "Bearer test")
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != tc.want || res.Header.Get("Access-Control-Allow-Origin") != origin || calls.Load() != int64(tc.calls) {
			t.Fatalf("%s %s: %d calls=%d body=%s", tc.method, tc.path, res.StatusCode, calls.Load(), body)
		}
		if tc.want == 204 && len(body) != 0 {
			t.Fatal("nonempty preflight")
		}
		if tc.want == 401 && !strings.Contains(string(body), "unauthorized") {
			t.Fatal("authentication response lost")
		}
	}
}
