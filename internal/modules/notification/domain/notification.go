package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid notification input")
	ErrForbidden   = errors.New("notification forbidden")
	ErrNotFound    = errors.New("notification not found")
	ErrUnavailable = errors.New("notifications unavailable")
)

type Notification struct {
	SourceType     string     `json:"source_type"`
	LeaveID        string     `json:"leave_id,omitempty"`
	NotificationID string     `json:"notification_id"`
	ComplaintID    string     `json:"complaint_id,omitempty"`
	Kind           string     `json:"kind"`
	Category       string     `json:"category,omitempty"`
	Status         string     `json:"status"`
	Title          string     `json:"title"`
	OccurredAt     time.Time  `json:"occurred_at"`
	CreatedAt      time.Time  `json:"created_at"`
	ReadAt         *time.Time `json:"read_at"`
}
type Page struct {
	Items   []Notification `json:"items"`
	Total   int64          `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	HasMore bool           `json:"has_more"`
}
type Unread struct {
	Unread int64 `json:"unread"`
}
