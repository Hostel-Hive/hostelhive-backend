package dto

import "time"

type LeaveQuery struct {
	From, To, StudentID, Status string
	Limit, Offset               int
}
type LeaveFilter struct {
	LeaveQuery
	Start, End time.Time
	Timezone   string
}
