package leave

import (
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc, roles ...string) http.Handler {
		return authenticate(middleware.RequireRoles(roles...)(h))
	}
	mux.Handle("POST /api/v1/leaves/{leaveID}/cancel", protect(a.Cancel, identity.RoleStudent))
	mux.Handle("POST /api/v1/leaves", protect(a.Submit, identity.RoleStudent))
	mux.Handle("GET /api/v1/leaves", protect(a.List, identity.RoleStudent, identity.RoleAdmin, identity.RoleWarden))
	mux.Handle("GET /api/v1/leaves/{leaveID}", protect(a.Get, identity.RoleStudent, identity.RoleAdmin, identity.RoleWarden))
	mux.Handle("POST /api/v1/leaves/{leaveID}/decision", protect(a.Decide, identity.RoleAdmin, identity.RoleWarden))
}

func RegisterGoingHome(a *handler.GoingHomeAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/going-home", authenticate(middleware.RequireRoles(identity.RoleStudent)(http.HandlerFunc(a.Declare))))
}
