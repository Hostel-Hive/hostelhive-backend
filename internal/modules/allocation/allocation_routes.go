package allocation

import (
	"net/http"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/handler"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(middleware.RequireRoles(identity.RoleAdmin, identity.RoleWarden)(h))
	}
	mux.Handle("GET /api/v1/allocations", protect(a.List))
	mux.Handle("POST /api/v1/allocations", protect(a.Assign))
	mux.Handle("POST /api/v1/allocations/{allocationID}/transfer", protect(a.Transfer))
	mux.Handle("POST /api/v1/allocations/{allocationID}/revoke", protect(a.Revoke))
}
