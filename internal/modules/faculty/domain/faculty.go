package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid faculty request")
	ErrForbidden   = errors.New("faculty access forbidden")
	ErrNotFound    = errors.New("faculty not found")
	ErrConflict    = errors.New("faculty name reserved")
	ErrUnavailable = errors.New("faculty storage unavailable")
)

type Faculty struct {
	FacultyID string    `json:"faculty_id"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Filter struct {
	Limit, Offset int
	Active        *bool
}
type Page struct {
	Items   []Faculty `json:"items"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"has_more"`
}
