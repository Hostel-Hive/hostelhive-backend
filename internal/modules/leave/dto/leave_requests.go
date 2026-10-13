package dto

import "time"

type Submit struct {
	RequestID string    `json:"request_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Reason    string    `json:"reason"`
}
type Decision struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}
