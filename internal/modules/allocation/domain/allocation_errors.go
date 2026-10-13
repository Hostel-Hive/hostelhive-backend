package domain

import "errors"

var (
	ErrInvalid     = errors.New("invalid allocation input")
	ErrNotFound    = errors.New("allocation, student or bed not found")
	ErrConflict    = errors.New("allocation conflicts with current state")
	ErrIneligible  = errors.New("active student profile and account required")
	ErrForbidden   = errors.New("allocation permission denied")
	ErrUnavailable = errors.New("allocation persistence unavailable")
)
