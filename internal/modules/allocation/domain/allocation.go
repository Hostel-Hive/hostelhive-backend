package domain

import "time"

type Allocation struct {
	AllocationID string     `json:"allocation_id"`
	StudentID    string     `json:"student_id"`
	BedID        string     `json:"bed_id"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at"`
	StartedBy    *string    `json:"started_by"`
	EndedBy      *string    `json:"ended_by"`
	EndReason    *string    `json:"end_reason"`
}
type Filter struct {
	StudentID, BedID string
	Active           *bool
	Limit, Offset    int
}
type Page struct {
	Items  []Allocation `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}
