package report

import (
	"net/http"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

func RegisterAttendance(a *handler.AttendanceAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	readers := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(identity.RoleAdmin, identity.RoleWarden)(h))
	}
	mux.Handle("GET /api/v1/reports/attendance", readers(a.Movements))
	mux.Handle("GET /api/v1/reports/attendance/daily", readers(a.Daily))
}

func RegisterLeaves(a *handler.LeaveAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/reports/leaves", authenticate(middleware.RequireRoles(identity.RoleAdmin, identity.RoleWarden)(http.HandlerFunc(a.Requests))))
}
