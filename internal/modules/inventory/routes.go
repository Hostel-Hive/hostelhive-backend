package inventory

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(domain.RoleAdmin, domain.RoleWarden)(h))
	}
	mux.Handle("GET /api/v1/blocks", protect(a.Blocks))
	mux.Handle("GET /api/v1/rooms", protect(a.Rooms))
	mux.Handle("GET /api/v1/beds", protect(a.Beds))
	admin := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(domain.RoleAdmin)(h))
	}
	mux.Handle("POST /api/v1/blocks", admin(a.CreateBlock))
	mux.Handle("PUT /api/v1/blocks/{blockID}", admin(a.UpdateBlock))
	mux.Handle("POST /api/v1/rooms", admin(a.CreateRoom))
	mux.Handle("PUT /api/v1/rooms/{roomID}", admin(a.UpdateRoom))
	mux.Handle("POST /api/v1/beds", admin(a.CreateBed))
	mux.Handle("PUT /api/v1/beds/{bedID}", admin(a.UpdateBed))
}
