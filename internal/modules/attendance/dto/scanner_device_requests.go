package dto

import "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"

type RegisterDevice struct {
	Name string `json:"name"`
}

// IssuedCredential is returned only on registration or rotation. Persistence
// never stores Credential, and ordinary device listings cannot return it.
type IssuedCredential struct {
	Device     domain.Device `json:"device"`
	Credential string        `json:"credential"`
}

type DeviceCheck struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}
