package dto

type Publish struct {
	RequestID string `json:"request_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Audience  string `json:"audience,omitempty"`
}
type Filter struct{ Limit, Offset int }
