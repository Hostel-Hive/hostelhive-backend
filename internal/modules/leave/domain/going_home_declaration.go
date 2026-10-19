package domain

import (
	"errors"
	"time"
)

var (
	ErrAlreadyOutside   = errors.New("student already outside")
	ErrPresenceUnknown  = errors.New("student presence unknown")
	ErrGoingHomePending = errors.New("outstanding going-home declaration")
)

type GoingHomeDeclaration struct {
	GoingHomeID       string     `json:"going_home_id"`
	RequestID         string     `json:"request_id"`
	StudentID         string     `json:"student_id"`
	SubmittedBy       string     `json:"submitted_by"`
	Status            string     `json:"status"`
	DeclaredAt        time.Time  `json:"declared_at"`
	DepartedAt        *time.Time `json:"departed_at"`
	DepartureDeviceID *string    `json:"departure_device_id"`
	DepartureEventID  *string    `json:"departure_event_id"`
}
