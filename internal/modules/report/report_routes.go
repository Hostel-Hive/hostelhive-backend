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

func RegisterComplaints(a *handler.ComplaintAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/reports/complaints", authenticate(middleware.RequireRoles(identity.RoleAdmin, identity.RoleWarden)(http.HandlerFunc(a.Records))))
}

func RegisterExports(a *handler.ExportAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	readers := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(identity.RoleAdmin, identity.RoleWarden)(h))
	}
	mux.Handle("GET /api/v1/reports/attendance/export", readers(a.Movements))
	mux.Handle("GET /api/v1/reports/attendance/daily/export", readers(a.Daily))
	mux.Handle("GET /api/v1/reports/leaves/export", readers(a.Leaves))
	mux.Handle("GET /api/v1/reports/complaints/export", readers(a.Complaints))
}

func RegisterPDFExports(a *handler.ExportAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	readers := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(identity.RoleAdmin, identity.RoleWarden)(h))
	}
	mux.Handle("GET /api/v1/reports/attendance/export/pdf", readers(a.Movements))
	mux.Handle("GET /api/v1/reports/attendance/daily/export/pdf", readers(a.Daily))
	mux.Handle("GET /api/v1/reports/leaves/export/pdf", readers(a.Leaves))
	mux.Handle("GET /api/v1/reports/complaints/export/pdf", readers(a.Complaints))
}
