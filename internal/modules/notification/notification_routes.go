package notification

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles("admin", "warden", "sub_warden", "student")(h))
	}
	mux.Handle("GET /api/v1/notifications", protect(a.List))
	mux.Handle("GET /api/v1/notifications/unread-count", protect(a.Unread))
	mux.Handle("POST /api/v1/notifications/{notificationID}/read", protect(a.MarkRead))
}

func RegisterPush(a *handler.PushAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("PUT /api/v1/me/push-devices/{deviceID}", authenticate(http.HandlerFunc(a.Register)))
	mux.Handle("GET /api/v1/me/push-devices", authenticate(http.HandlerFunc(a.List)))
	mux.Handle("DELETE /api/v1/me/push-devices/{deviceID}", authenticate(http.HandlerFunc(a.Remove)))
}
