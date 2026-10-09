package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalid       = errors.New("invalid staff input")
	ErrNotFound      = errors.New("staff profile or account not found")
	ErrForbidden     = errors.New("staff management forbidden")
	ErrStaffRequired = errors.New("staff role required")
	ErrUnavailable   = errors.New("staff profiles unavailable")
)

type Profile struct {
	StaffID     string    `json:"staff_id"`
	UserID      string    `json:"user_id"`
	FullName    string    `json:"full_name"`
	Designation string    `json:"designation"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Page struct {
	Staff   []Profile `json:"staff"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"has_more"`
}
