package dto

import student "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"

type StudentInvitationInput struct {
	RequestID string `json:"request_id"`
	Email     string `json:"email"`
	student.Details
}
type InvitationResend struct {
	RequestID string `json:"request_id"`
}
