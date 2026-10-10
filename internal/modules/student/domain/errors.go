package domain

import "errors"

var (
	ErrInvalid     = errors.New("invalid input")
	ErrNotFound    = errors.New("student not found")
	ErrConflict    = errors.New("duplicate student")
	ErrAccount     = errors.New("active student account required")
	ErrForbidden   = errors.New("forbidden")
	ErrUnavailable = errors.New("student profiles unavailable")
)
