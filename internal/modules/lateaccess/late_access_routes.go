package lateaccess

import (
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc, roles ...string) http.Handler {
		return authenticate(middleware.RequireRoles(roles...)(h))
	}
	mux.Handle("POST /api/v1/late-access-requests", protect(a.Submit, identity.RoleStudent))
	mux.Handle("GET /api/v1/late-access-requests", protect(a.List, identity.RoleStudent, identity.RoleAdmin, identity.RoleWarden))
	mux.Handle("GET /api/v1/late-access-requests/{lateAccessID}", protect(a.Get, identity.RoleStudent, identity.RoleAdmin, identity.RoleWarden))
	mux.Handle("POST /api/v1/late-access-requests/{lateAccessID}/decision", protect(a.Decide, identity.RoleAdmin, identity.RoleWarden))
}
