package server

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/accountmanagement"
	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
)

func newHandler(ping func(context.Context) error, timeout time.Duration, protected ...func(http.Handler) http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "{\"status\":\"ok\"}\n")
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		if err := ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "{\"status\":\"not_ready\"}\n")
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "{\"status\":\"ready\"}\n")
	})
	if len(protected) == 1 {
		mux.Handle("GET /api/v1/me", protected[0](http.HandlerFunc(authentication.Me)))
	}
	return mux
}

func newAPIHandler(ping func(context.Context) error, timeout time.Duration, authenticate func(http.Handler) http.Handler, users http.Handler, management ...*accountmanagement.API) http.Handler {
	mux := newHandler(ping, timeout, authenticate)
	mux.Handle("POST /api/v1/users", authenticate(authentication.RequireRoles(authentication.RoleAdmin)(users)))
	if len(management) == 1 {
		protect := func(h http.HandlerFunc) http.Handler {
			return authenticate(authentication.RequireRoles(authentication.RoleAdmin)(h))
		}
		mux.Handle("GET /api/v1/users", protect(management[0].List))
		mux.Handle("PATCH /api/v1/users/{userID}/role", protect(management[0].Role))
		mux.Handle("POST /api/v1/users/{userID}/deactivate", protect(management[0].Deactivate))
	}
	return mux
}
