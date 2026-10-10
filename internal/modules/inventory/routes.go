package inventory

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(domain.RoleWarden)(h))
	}
	mux.Handle("GET /api/v1/blocks", protect(a.Blocks))
	mux.Handle("GET /api/v1/rooms", protect(a.Rooms))
	mux.Handle("GET /api/v1/beds", protect(a.Beds))
}
