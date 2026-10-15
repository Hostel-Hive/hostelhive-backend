package dto

import "time"

// AttendanceQuery is transport input; AttendanceFilter holds validated local
// calendar dates and a half-open absolute time range for repository reads.
type AttendanceQuery struct {
	From, To, StudentID, Direction string
	Limit, Offset                  int
}
type AttendanceFilter struct {
	AttendanceQuery
	Start, End time.Time
	Timezone   string
	Days       int
}
