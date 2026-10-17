package domain

import (
	"errors"
	"time"
)

const PushDeviceLimit = 10
const PushMaxAttempts = 8

var ErrPushConflict = errors.New("push device registration conflicts")

type PushDevice struct {
	DeviceID  string    `json:"device_id"`
	Platform  string    `json:"platform"`
	Active    bool      `json:"active"`
	UpdatedAt time.Time `json:"updated_at"`
}
type PushDevicePage struct {
	Items []PushDevice `json:"items"`
	Total int          `json:"total"`
}

// Token is confidential provider routing material, never an API response field.
type PushMessage struct {
	Token          string
	NotificationID string
	SourceType     string
	Kind           string
	ExpiresAt      time.Time
}
type PushResult struct {
	State      string // accepted, retry, invalid_token, failed
	ProviderID string
	Code       string // safe fixed code; never raw provider error text
	RetryAfter time.Duration
}
