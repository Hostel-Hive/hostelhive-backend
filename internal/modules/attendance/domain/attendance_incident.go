package domain

import "time"

const PermissionWindow = 30 * time.Minute

func PermissionBounds(at time.Time) (time.Time, time.Time) {
	return at.Add(-PermissionWindow), at.Add(PermissionWindow)
}

type Incident struct {
	IncidentID              string    `json:"incident_id"`
	DeviceID                string    `json:"device_id"`
	EventID                 string    `json:"event_id"`
	StudentID               string    `json:"student_id"`
	Direction               string    `json:"direction"`
	IncidentType            string    `json:"incident_type"`
	OccurredAt              time.Time `json:"occurred_at"`
	HostelTimezone          string    `json:"hostel_timezone"`
	ExitOpeningMinute       int       `json:"exit_opening_minute"`
	EntryDeadlineMinute     int       `json:"entry_deadline_minute"`
	PermissionWindowSeconds int       `json:"permission_window_seconds"`
}
type IncidentPage struct {
	Items   []Incident `json:"items"`
	Total   int64      `json:"total"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
	HasMore bool       `json:"has_more"`
}
