package announcement

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc, roles ...string) http.Handler {
		return authenticate(middleware.RequireRoles(roles...)(h))
	}
	mux.Handle("POST /api/v1/announcements", protect(a.Publish, "admin", "warden"))
	mux.Handle("GET /api/v1/announcements", protect(a.List, "admin", "warden", "student"))
	mux.Handle("GET /api/v1/announcements/{announcementID}", protect(a.Get, "admin", "warden", "student"))
	mux.Handle("GET /api/v1/announcements/{announcementID}/delivery-summary", protect(a.Summary, "admin", "warden"))
}
