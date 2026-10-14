package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid complaint input")
	ErrForbidden   = errors.New("complaint forbidden")
	ErrNotFound    = errors.New("complaint not found")
	ErrConflict    = errors.New("complaint conflict")
	ErrIneligible  = errors.New("active student required")
	ErrUnavailable = errors.New("complaint unavailable")
)

type Complaint struct {
	ComplaintID string    `json:"complaint_id"`
	RequestID   string    `json:"request_id"`
	StudentID   string    `json:"student_id"`
	SubmittedBy string    `json:"submitted_by"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Update struct {
	ImageID         *string   `json:"image_id,omitempty"`
	UpdateID        string    `json:"update_id"`
	ComplaintID     string    `json:"complaint_id"`
	RequestID       string    `json:"request_id"`
	ActorID         string    `json:"actor_id"`
	Action          string    `json:"action"`
	PreviousStatus  *string   `json:"previous_status"`
	NewStatus       string    `json:"new_status"`
	Text            string    `json:"text"`
	PreviousVersion int64     `json:"previous_version"`
	Version         int64     `json:"version"`
	OccurredAt      time.Time `json:"occurred_at"`
}
type Filter struct {
	StudentID, Category, Status string
	From, To                    *time.Time
	Limit, Offset               int
}
type Page struct {
	Items   []Complaint `json:"items"`
	Total   int64       `json:"total"`
	Limit   int         `json:"limit"`
	Offset  int         `json:"offset"`
	HasMore bool        `json:"has_more"`
}
type UpdatePage struct {
	Items   []Update `json:"items"`
	Total   int64    `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
	HasMore bool     `json:"has_more"`
}
