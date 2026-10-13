package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid leave input")
	ErrForbidden   = errors.New("leave forbidden")
	ErrNotFound    = errors.New("leave not found")
	ErrIneligible  = errors.New("active student required")
	ErrConflict    = errors.New("leave conflict")
	ErrUnavailable = errors.New("leave unavailable")
)

type Request struct {
	LeaveID      string     `json:"leave_id"`
	RequestID    string     `json:"request_id"`
	StudentID    string     `json:"student_id"`
	SubmittedBy  string     `json:"submitted_by"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       time.Time  `json:"ends_at"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	DecidedAt    *time.Time `json:"decided_at"`
	DecidedBy    *string    `json:"decided_by"`
	DecisionNote string     `json:"decision_note"`
}
type Filter struct {
	StudentID, Status string
	Limit, Offset     int
}
type Page struct {
	Items   []Request `json:"items"`
	Total   int64     `json:"total"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"has_more"`
}
