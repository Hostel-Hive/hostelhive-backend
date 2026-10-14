package domain

import (
	"errors"
	"time"
)

var (
	ErrDirection     = errors.New("scanner direction not configured")
	ErrStudent       = errors.New("active student QR required")
	ErrEventConflict = errors.New("event ID reused with different payload")
)

type Event struct {
	AccessResult string    `json:"access_result,omitempty"`
	LateAccessID *string   `json:"late_access_id,omitempty"`
	IncidentID   *string   `json:"incident_id,omitempty"`
	EventID      string    `json:"event_id"`
	DeviceID     string    `json:"device_id"`
	StudentID    string    `json:"student_id"`
	Direction    string    `json:"direction"`
	Outcome      string    `json:"outcome"`
	OccurredAt   time.Time `json:"occurred_at"`
}

type Presence struct {
	StudentID string     `json:"student_id"`
	Status    string     `json:"status"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type EventPage struct {
	Items   []Event `json:"items"`
	Limit   int     `json:"limit"`
	Offset  int     `json:"offset"`
	HasMore bool    `json:"has_more"`
}
