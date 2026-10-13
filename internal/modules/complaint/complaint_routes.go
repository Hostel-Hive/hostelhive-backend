package complaint

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/handler"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc, roles ...string) http.Handler {
		return authenticate(middleware.RequireRoles(roles...)(h))
	}
	all := []string{identity.RoleStudent, identity.RoleAdmin, identity.RoleWarden, identity.RoleSubWarden}
	mux.Handle("POST /api/v1/complaints", protect(a.Submit, identity.RoleStudent))
	mux.Handle("GET /api/v1/complaints", protect(a.List, all...))
	mux.Handle("GET /api/v1/complaints/{complaintID}", protect(a.Get, all...))
	mux.Handle("GET /api/v1/complaints/{complaintID}/updates", protect(a.Updates, all...))
	mux.Handle("POST /api/v1/complaints/{complaintID}/notes", protect(a.Note, all...))
	mux.Handle("POST /api/v1/complaints/{complaintID}/status", protect(a.Status, identity.RoleAdmin, identity.RoleWarden, identity.RoleSubWarden))
}
