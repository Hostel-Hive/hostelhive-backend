package domain

// Import results identify CSV records without echoing personal information.
type ImportRowResult struct {
	Record    int    `json:"record"`
	Line      int    `json:"line"`
	Status    string `json:"status"`
	StudentID string `json:"student_id,omitempty"`
	Error     string `json:"error,omitempty"`
}
type ImportReport struct {
	Total         int               `json:"total"`
	Created       int               `json:"created"`
	Rejected      int               `json:"rejected"`
	NotAttempted  int               `json:"not_attempted"`
	StoppedReason string            `json:"stopped_reason,omitempty"`
	Rows          []ImportRowResult `json:"rows"`
}
