package domain

import "errors"

var ErrInvalid = errors.New("invalid inventory filter")

type Filter struct {
	BlockID   string
	RoomID    string
	Available *bool
	Limit     int
	Offset    int
}
type Block struct {
	BlockID       string `json:"block_id"`
	Name          string `json:"name"`
	RoomCount     int64  `json:"room_count"`
	Capacity      int64  `json:"capacity"`
	OccupiedBeds  int64  `json:"occupied_beds"`
	AvailableBeds int64  `json:"available_beds"`
}
type Room struct {
	RoomID        string `json:"room_id"`
	BlockID       string `json:"block_id"`
	RoomNo        string `json:"room_no"`
	Capacity      int64  `json:"capacity"`
	OccupiedBeds  int64  `json:"occupied_beds"`
	AvailableBeds int64  `json:"available_beds"`
}
type Bed struct {
	BedID     string `json:"bed_id"`
	RoomID    string `json:"room_id"`
	BlockID   string `json:"block_id"`
	BedNo     string `json:"bed_no"`
	Available bool   `json:"available"`
}

// Total and items are read from the same PostgreSQL snapshot.
type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}
