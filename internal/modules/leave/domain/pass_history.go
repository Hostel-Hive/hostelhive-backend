package domain

import "time"

// PassItem is a read projection. Source IDs still address the existing workflows.
type PassItem struct {
	Type         string     `json:"type"`
	SourceID     string     `json:"source_id"`
	RequestID    string     `json:"request_id"`
	StudentID    string     `json:"student_id"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	Direction    *string    `json:"direction"`
	ExpectedAt   *time.Time `json:"expected_at"`
	DecidedAt    *time.Time `json:"decided_at"`
	DecidedBy    *string    `json:"decided_by"`
	DecisionNote string     `json:"decision_note"`
	CancelledAt  *time.Time `json:"cancelled_at"`
	CancelledBy  *string    `json:"cancelled_by"`
	ConsumedAt   *time.Time `json:"consumed_at"`
}

type PassFilter struct {
	Type, Status  string
	Limit, Offset int
}

type PassPage struct {
	Items   []PassItem `json:"items"`
	Total   int64      `json:"total"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
	HasMore bool       `json:"has_more"`
}
