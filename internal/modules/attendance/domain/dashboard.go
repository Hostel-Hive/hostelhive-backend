package domain

import "time"

type PresenceStudent struct {
	StudentID string     `json:"student_id"`
	IndexNo   string     `json:"index_no"`
	FullName  string     `json:"full_name"`
	Status    string     `json:"status"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type PresencePage struct {
	Items   []PresenceStudent `json:"items"`
	Total   int64             `json:"total"`
	Limit   int               `json:"limit"`
	Offset  int               `json:"offset"`
	HasMore bool              `json:"has_more"`
}

type PresenceSummary struct {
	Total   int64 `json:"total"`
	In      int64 `json:"in"`
	Out     int64 `json:"out"`
	Unknown int64 `json:"unknown"`
}
