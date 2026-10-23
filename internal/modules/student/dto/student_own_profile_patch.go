package dto

import "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"

// Nil means omitted, not cleared. The HTTP decoder rejects explicit nulls.
// When supplied, Guardians replaces the complete list; clients do not supply IDs.
type OwnProfilePatch struct {
	ContactPhone *string                 `json:"contact_phone,omitempty"`
	Guardians    *[]domain.GuardianInput `json:"guardians,omitempty"`
}
