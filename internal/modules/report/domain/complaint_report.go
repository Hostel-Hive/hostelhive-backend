package domain

import "time"

// Resolution is the recorded status update that produced the current resolved state.
// Reopened complaints and legacy records without a resolution update use null.
type ComplaintResolution struct {
	UpdateID   string    `json:"update_id"`
	ResolvedAt time.Time `json:"resolved_at"`
	ResolvedBy string    `json:"resolved_by"`
	Note       string    `json:"note"`
}
type ComplaintRecord struct {
	ComplaintID string               `json:"complaint_id"`
	RequestID   string               `json:"request_id"`
	StudentID   string               `json:"student_id"`
	SubmittedBy string               `json:"submitted_by"`
	Category    string               `json:"category"`
	Description string               `json:"description"`
	Status      string               `json:"status"`
	Version     int64                `json:"version"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Resolution  *ComplaintResolution `json:"resolution"`
}
type ComplaintStatuses struct {
	Pending    int64 `json:"pending"`
	InProgress int64 `json:"in_progress"`
	Resolved   int64 `json:"resolved"`
}
type ComplaintCategories struct {
	Maintenance  int64 `json:"maintenance"`
	Disciplinary int64 `json:"disciplinary"`
}
type ComplaintTotals struct {
	ByStatus   ComplaintStatuses   `json:"by_status"`
	ByCategory ComplaintCategories `json:"by_category"`
}
type ComplaintPage struct {
	From      string            `json:"from"`
	To        string            `json:"to"`
	Timezone  string            `json:"timezone"`
	StudentID string            `json:"student_id,omitempty"`
	Category  string            `json:"category,omitempty"`
	Status    string            `json:"status,omitempty"`
	Summary   ComplaintTotals   `json:"summary"`
	Total     int64             `json:"total"`
	Limit     int               `json:"limit"`
	Offset    int               `json:"offset"`
	HasMore   bool              `json:"has_more"`
	Items     []ComplaintRecord `json:"items"`
}
