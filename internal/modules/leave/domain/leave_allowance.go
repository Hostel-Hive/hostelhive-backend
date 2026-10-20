package domain

import "time"

// AllowancePeriod is a bounded calendar year, using an exclusive End instant.
type AllowancePeriod struct {
	Year       int
	Start, End time.Time
	Timezone   string
}

type AllowanceUsage struct {
	StudentID string
	UsedDays  int
}

type AllowanceSummary struct {
	StudentID     string `json:"student_id"`
	Year          int    `json:"year"`
	Timezone      string `json:"timezone"`
	TotalDays     int    `json:"total_days"`
	UsedDays      int    `json:"used_days"`
	RemainingDays int    `json:"remaining_days"`
	ExceededDays  int    `json:"exceeded_days"`
}
