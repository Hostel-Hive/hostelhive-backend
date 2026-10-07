package middleware

import (
	"net/http"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	validPolicy := len(roles) > 0
	for _, role := range roles {
		if !domain.ValidRole(role) {
			validPolicy = false
		}
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			account, ok := AccountFromContext(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			_, permitted := allowed[account.Role]
			if !validPolicy || !account.IsActive || !domain.ValidRole(account.Role) || !permitted {
				response.Error(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
