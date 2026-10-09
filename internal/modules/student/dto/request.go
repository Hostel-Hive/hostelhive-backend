package dto

import (
	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
)

type Details struct {
	IndexNo      string                  `json:"index_no"`
	FullName     string                  `json:"full_name"`
	Faculty      string                  `json:"faculty"`
	Year         int                     `json:"year"`
	ContactPhone string                  `json:"contact_phone"`
	Guardians    []sdomain.GuardianInput `json:"guardians"`
}

type CreateInput struct {
	UserID string `json:"user_id"`
	Details
}
