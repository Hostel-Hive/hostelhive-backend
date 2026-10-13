package dto

type Submit struct {
	RequestID   string `json:"request_id"`
	Category    string `json:"category"`
	Description string `json:"description"`
}
type Change struct {
	RequestID       string `json:"request_id"`
	ExpectedVersion int64  `json:"expected_version"`
	Status          string `json:"status,omitempty"`
	Text            string `json:"text"`
}
