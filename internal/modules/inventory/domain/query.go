package domain

type Filter struct {
	BlockID   string
	RoomID    string
	Available *bool
	Limit     int
	Offset    int
}

// Total and items are read from the same PostgreSQL snapshot.
type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}
