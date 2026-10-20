package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid late-access input")
	ErrForbidden   = errors.New("late access forbidden")
	ErrNotFound    = errors.New("late access not found")
	ErrIneligible  = errors.New("active student required")
	ErrConflict    = errors.New("late access conflict")
	ErrUnavailable = errors.New("late access unavailable")
)

type Request struct {
	CancelledAt      *time.Time `json:"cancelled_at"`
	CancelledBy      *string    `json:"cancelled_by"`
	ConsumedAt       *time.Time `json:"consumed_at,omitempty"`
	ConsumedDeviceID *string    `json:"consumed_device_id,omitempty"`
	ConsumedEventID  *string    `json:"consumed_event_id,omitempty"`
	LateAccessID     string     `json:"late_access_id"`
	RequestID        string     `json:"request_id"`
	StudentID        string     `json:"student_id"`
	SubmittedBy      string     `json:"submitted_by"`
	Direction        string     `json:"direction"`
	ExpectedAt       time.Time  `json:"expected_at"`
	Reason           string     `json:"reason"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	DecidedAt        *time.Time `json:"decided_at"`
	DecidedBy        *string    `json:"decided_by"`
	DecisionNote     string     `json:"decision_note"`
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

// Policy uses hostel local wall time, independent of the submitted offset.
// Entry after midnight belongs to the overnight restriction until exits open.
type Policy struct {
	Location                   *time.Location
	ExitOpening, EntryDeadline int
}

func (p Policy) RequiresPermission(direction string, at time.Time) bool {
	local := at.In(p.Location)
	minute := local.Hour()*60 + local.Minute()
	if direction == "OUT" {
		return minute < p.ExitOpening
	}
	return direction == "IN" && (minute >= p.EntryDeadline || minute < p.ExitOpening)
}
