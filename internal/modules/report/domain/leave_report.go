package domain

import "time"

type LeaveRequest struct {
	CancelledAt  *time.Time `json:"cancelled_at"`
	CancelledBy  *string    `json:"cancelled_by"`
	LeaveID      string     `json:"leave_id"`
	RequestID    string     `json:"request_id"`
	StudentID    string     `json:"student_id"`
	SubmittedBy  string     `json:"submitted_by"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       time.Time  `json:"ends_at"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	DecidedAt    *time.Time `json:"decided_at"`
	DecidedBy    *string    `json:"decided_by"`
	DecisionNote string     `json:"decision_note"`
}
type LeaveTotals struct {
	Cancelled int64 `json:"cancelled"`
	Pending   int64 `json:"pending"`
	Approved  int64 `json:"approved"`
	Rejected  int64 `json:"rejected"`
}
type LeavePage struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Timezone  string         `json:"timezone"`
	StudentID string         `json:"student_id,omitempty"`
	Status    string         `json:"status,omitempty"`
	Summary   LeaveTotals    `json:"summary"`
	Total     int64          `json:"total"`
	Limit     int            `json:"limit"`
	Offset    int            `json:"offset"`
	HasMore   bool           `json:"has_more"`
	Items     []LeaveRequest `json:"items"`
}
