package authentication

import "net/http"

// Roles match the user-account migration. They do not imply a permission
// hierarchy: every endpoint must explicitly list each permitted role.
const (
	RoleAdmin         = "admin"
	RoleWarden        = "warden"
	RoleSubWarden     = "sub_warden"
	RoleSecurityStaff = "security_staff"
	RoleStudent       = "student"
)

// RequireRoles must run after authentication Middleware. An empty policy or an
// unsupported configured role denies all access rather than weakening a policy.
// The policy is copied at construction; later caller mutations cannot change it.
func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	validPolicy := len(roles) > 0
	for _, role := range roles {
		if !validRole(role) {
			validPolicy = false
		}
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			account, ok := AccountFromContext(r.Context())
			if !ok {
				reject(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			_, permitted := allowed[account.Role]
			if !validPolicy || !account.IsActive || !validRole(account.Role) || !permitted {
				reject(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
