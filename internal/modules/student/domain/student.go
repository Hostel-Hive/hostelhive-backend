package domain

import (
	"errors"

	"time"
)

var (
	ErrInvalid     = errors.New("invalid input")
	ErrNotFound    = errors.New("student not found")
	ErrConflict    = errors.New("duplicate student")
	ErrAccount     = errors.New("active student account required")
	ErrForbidden   = errors.New("forbidden")
	ErrUnavailable = errors.New("student profiles unavailable")
)

type GuardianInput struct {
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	ContactPhone string `json:"contact_phone"`
}

type Guardian struct {
	GuardianID string `json:"guardian_id"`
	GuardianInput
}

type Profile struct {
	StudentID     string     `json:"student_id"`
	UserID        string     `json:"user_id"`
	IndexNo       string     `json:"index_no"`
	FullName      string     `json:"full_name"`
	Faculty       string     `json:"faculty"`
	Year          int        `json:"year"`
	ContactPhone  string     `json:"contact_phone"`
	Email         string     `json:"email"`
	AccountActive bool       `json:"account_active"`
	Guardians     []Guardian `json:"guardians,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Filter struct {
	Limit, Offset, Year int
	Q, Faculty          string
}

type Page struct {
	Students []Profile `json:"students"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
	HasMore  bool      `json:"has_more"`
}
