package dto

import "time"

type ComplaintQuery struct {
	From, To, StudentID, Category, Status string
	Limit, Offset                         int
}
type ComplaintFilter struct {
	ComplaintQuery
	Start, End time.Time
	Timezone   string
}
