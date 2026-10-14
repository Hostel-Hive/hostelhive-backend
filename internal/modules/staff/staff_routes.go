package staff

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(domain.RoleAdmin)(h))
	}
	mux.Handle("PUT /api/v1/staff/me", authenticate(middleware.RequireRoles(domain.RoleAdmin, domain.RoleWarden, domain.RoleSubWarden, domain.RoleSecurityStaff)(http.HandlerFunc(a.PutSelf))))
	mux.Handle("GET /api/v1/staff", protect(a.List))
	mux.Handle("GET /api/v1/staff/{userID}", protect(a.Get))
	mux.Handle("PUT /api/v1/staff/{userID}", protect(a.Put))
}
