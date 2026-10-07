package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProtectedRouteAndPublicProbes(t *testing.T) {
	calls := 0
	protect := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusUnauthorized) })
	}
	handler := newHandler(func(context.Context) error { return nil }, time.Second, protect)
	for _, tc := range []struct {
		method, path string
		want         int
	}{{"GET", "/health", 200}, {"GET", "/ready", 200}, {"GET", "/api/v1/me", 401}, {"POST", "/api/v1/me", 405}, {"GET", "/api/v1/me/other", 404}} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.want {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
	if calls != 1 {
		t.Fatal("probe or unknown route used authentication")
	}
}
