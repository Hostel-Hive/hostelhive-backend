package domain

import "errors"

var (
	ErrInvalid      = errors.New("invalid scanner input")
	ErrUnauthorized = errors.New("invalid scanner credentials")
	ErrForbidden    = errors.New("scanner management forbidden")
	ErrNotFound     = errors.New("scanner not found")
	ErrConflict     = errors.New("scanner name conflict")
	ErrUnavailable  = errors.New("scanner persistence unavailable")
)
