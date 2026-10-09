package domain

import (
	"errors"
)

var ErrAccountNotFound = errors.New("account not found")

type Account struct {
	UserID      string `json:"user_id"`
	FirebaseUID string `json:"firebase_uid"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
}

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

func ValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleWarden, RoleSubWarden, RoleSecurityStaff, RoleStudent:
		return true
	}
	return false
}
