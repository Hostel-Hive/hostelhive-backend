package identity

import (
	"net/http"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	handler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/handler"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

// Register attaches the explicit administrator policy to identity-management routes.
func Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler, users http.Handler, management *handler.API) {
	mux.Handle("POST /api/v1/users", authenticate(authentication.RequireRoles(domain.RoleAdmin)(users)))
	if management != nil {
		protect := func(h http.HandlerFunc) http.Handler {
			return authenticate(authentication.RequireRoles(domain.RoleAdmin)(h))
		}
		mux.Handle("GET /api/v1/users", protect(management.List))
		mux.Handle("PATCH /api/v1/users/{userID}/role", protect(management.Role))
		mux.Handle("POST /api/v1/users/{userID}/deactivate", protect(management.Deactivate))
		mux.Handle("POST /api/v1/users/{userID}/activate", protect(management.Activate))
	}
}

func RegisterMe(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/me", authenticate(http.HandlerFunc(handler.Me)))
}
