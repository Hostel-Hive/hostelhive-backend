package dto

import "time"

type Submit struct {
	RequestID  string    `json:"request_id"`
	Direction  string    `json:"direction"`
	ExpectedAt time.Time `json:"expected_at"`
	Reason     string    `json:"reason"`
}
type Decision struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}
