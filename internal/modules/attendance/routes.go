package attendance

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(domain.RoleAdmin)(h))
	}
	mux.Handle("POST /api/v1/scanner-devices", protect(a.Register))
	mux.Handle("GET /api/v1/scanner-devices", protect(a.List))
	mux.Handle("POST /api/v1/scanner-devices/{deviceID}/rotate-credential", protect(a.Rotate))
	mux.Handle("POST /api/v1/scanner-devices/{deviceID}/deactivate", protect(a.Deactivate))
	mux.Handle("GET /api/v1/device/check", a.DeviceMiddleware(http.HandlerFunc(a.Check)))
}

func RegisterScans(a *handler.ScanAPI, devices *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("PUT /api/v1/scanner-devices/{deviceID}/direction", authenticate(middleware.RequireRoles(domain.RoleAdmin)(http.HandlerFunc(a.SetDirection))))
	mux.Handle("POST /api/v1/device/scans", devices.DeviceMiddleware(http.HandlerFunc(a.Record)))
	readers := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(domain.RoleAdmin, domain.RoleWarden)(h))
	}
	mux.Handle("GET /api/v1/attendance/presence", readers(a.Presence))
	mux.Handle("GET /api/v1/attendance/events", readers(a.Events))
}
