package domain

const (
	RoleAdmin         = "admin"
	RoleWarden        = "warden"
	RoleSubWarden     = "sub_warden"
	RoleSecurityStaff = "security_staff"
	RoleStudent       = "student"
)

func ValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleWarden, RoleSubWarden, RoleSecurityStaff, RoleStudent:
		return true
	}
	return false
}
