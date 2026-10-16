package domain

import "time"

// StudentMovement exposes recorded movement only, without scanner processing metadata.
type StudentMovement struct {
	MovementID string    `json:"movement_id"`
	EventID    string    `json:"event_id"`
	Direction  string    `json:"direction"`
	OccurredAt time.Time `json:"occurred_at"`
}
type StudentHistoryPage struct {
	Items    []StudentMovement `json:"items"`
	Total    int64             `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
	HasMore  bool              `json:"has_more"`
	Timezone string            `json:"timezone"`
	From     *string           `json:"from"`
	To       *string           `json:"to"`
}
