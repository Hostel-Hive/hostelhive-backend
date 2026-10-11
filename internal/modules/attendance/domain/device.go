package domain

import "time"

type Device struct {
	DeviceID          string    `json:"device_id"`
	Name              string    `json:"name"`
	Direction         *string   `json:"direction"`
	IsActive          bool      `json:"is_active"`
	CredentialVersion int       `json:"credential_version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	CreatedBy         string    `json:"created_by"`
	UpdatedBy         string    `json:"updated_by"`
}

type DevicePage struct {
	Items   []Device `json:"items"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
	HasMore bool     `json:"has_more"`
}

// AuthenticationMaterial is internal persistence data, never an API response.
type AuthenticationMaterial struct {
	Device Device `json:"-"`
	Hash   []byte `json:"-"`
}
