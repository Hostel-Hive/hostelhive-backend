package domain

import "errors"

var (
	ErrInvalid       = errors.New("invalid staff input")
	ErrNotFound      = errors.New("staff profile or account not found")
	ErrForbidden     = errors.New("staff management forbidden")
	ErrStaffRequired = errors.New("staff role required")
	ErrUnavailable   = errors.New("staff profiles unavailable")
)
