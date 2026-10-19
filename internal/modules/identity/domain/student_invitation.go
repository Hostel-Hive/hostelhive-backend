package domain

import (
	"errors"
	"time"
)

var (
	ErrInvitationInvalid     = errors.New("invalid invitation input")
	ErrInvitationConflict    = errors.New("invitation conflict")
	ErrInvitationForbidden   = errors.New("invitation forbidden")
	ErrInvitationNotFound    = errors.New("invitation not found")
	ErrInvitationUnavailable = errors.New("invitation unavailable")
	ErrInvitationRateLimited = errors.New("invitation resend rate limited")
	ErrSetupPending          = errors.New("password setup not verified")
	ErrInvitationIdentity    = errors.New("provider identity conflict")
)

type StudentInvitation struct {
	InvitationID     string     `json:"invitation_id"`
	RequestID        string     `json:"request_id"`
	UserID           string     `json:"user_id"`
	StudentID        string     `json:"student_id"`
	Email            string     `json:"email"`
	DeliveryStatus   string     `json:"delivery_status"`
	Attempts         int        `json:"attempts"`
	LastError        string     `json:"last_error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	LastRequestedAt  time.Time  `json:"last_requested_at"`
	NextAttemptAt    *time.Time `json:"next_attempt_at"`
	LastSentAt       *time.Time `json:"last_sent_at"`
	SetupCompletedAt *time.Time `json:"setup_completed_at"`
}
type InvitationPage struct {
	Items   []StudentInvitation `json:"items"`
	Total   int64               `json:"total"`
	Limit   int                 `json:"limit"`
	Offset  int                 `json:"offset"`
	HasMore bool                `json:"has_more"`
}
type SetupResult struct {
	SetupRequired    bool       `json:"setup_required"`
	SetupCompletedAt *time.Time `json:"setup_completed_at"`
}

// InvitationRetry contains only safe retry metadata, never a provider response.
type InvitationRetry struct{ After time.Duration }

func (e *InvitationRetry) Error() string { return "invitation provider unavailable" }
