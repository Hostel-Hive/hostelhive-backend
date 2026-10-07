package app

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity"
	identityhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/handler"
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
		identity.RegisterMe(mux, protected[0])
	}
	return mux
}

func newAPIHandler(ping func(context.Context) error, timeout time.Duration, authenticate func(http.Handler) http.Handler, users http.Handler, management ...*identityhandler.API) *http.ServeMux {
	mux := newHandler(ping, timeout, authenticate)
	var api *identityhandler.API
	if len(management) == 1 {
		api = management[0]
	}
	identity.Register(mux, authenticate, users, api)
	return mux
}
