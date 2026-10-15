package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid attendance report")
	ErrForbidden   = errors.New("report forbidden")
	ErrUnavailable = errors.New("report unavailable")
)

type Movement struct {
	EventID      string    `json:"event_id"`
	DeviceID     string    `json:"device_id"`
	StudentID    string    `json:"student_id"`
	Direction    string    `json:"direction"`
	OccurredAt   time.Time `json:"occurred_at"`
	AccessResult *string   `json:"access_result,omitempty"`
	LateAccessID *string   `json:"late_access_id,omitempty"`
	IncidentID   *string   `json:"incident_id,omitempty"`
}

type Totals struct {
	Entries          int64 `json:"entries"`
	Exits            int64 `json:"exits"`
	DistinctStudents int64 `json:"distinct_students"`
}

type Metadata struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Timezone  string `json:"timezone"`
	StudentID string `json:"student_id,omitempty"`
	Direction string `json:"direction,omitempty"`
	Summary   Totals `json:"summary"`
	Total     int64  `json:"total"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
	HasMore   bool   `json:"has_more"`
}
type MovementPage struct {
	Metadata
	Items []Movement `json:"items"`
}
type Day struct {
	Date string `json:"date"`
	Totals
}
type DailyPage struct {
	Metadata
	Items []Day `json:"items"`
}
