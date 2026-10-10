package domain

import "errors"

var ErrInvalid = errors.New("invalid inventory filter")

var (
	ErrNotFound    = errors.New("inventory resource not found")
	ErrConflict    = errors.New("duplicate inventory name or number")
	ErrForbidden   = errors.New("inventory modification forbidden")
	ErrUnavailable = errors.New("inventory persistence unavailable")
)
