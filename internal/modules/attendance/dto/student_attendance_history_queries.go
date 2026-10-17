package dto

import "time"

type StudentHistoryQuery struct {
	From, To, Direction string
	Limit, Offset       int
}
type StudentHistoryFilter struct {
	StudentHistoryQuery
	Start, End *time.Time
	Timezone   string
}
