package faculty

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	readers := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles("admin", "warden")(h))
	}
	admin := func(h http.HandlerFunc) http.Handler { return authenticate(middleware.RequireRoles("admin")(h)) }
	mux.Handle("GET /api/v1/faculties", readers(a.List))
	mux.Handle("GET /api/v1/faculties/{facultyID}", readers(a.Get))
	mux.Handle("POST /api/v1/faculties", admin(a.Create))
	mux.Handle("PATCH /api/v1/faculties/{facultyID}", admin(a.Rename))
	mux.Handle("POST /api/v1/faculties/{facultyID}/deactivate", admin(a.Deactivate))
	mux.Handle("POST /api/v1/faculties/{facultyID}/activate", admin(a.Activate))
}
