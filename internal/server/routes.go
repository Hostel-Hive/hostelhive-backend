package server

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
)

func newHandler(ping func(context.Context) error, timeout time.Duration, protected ...func(http.Handler) http.Handler) http.Handler {
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
