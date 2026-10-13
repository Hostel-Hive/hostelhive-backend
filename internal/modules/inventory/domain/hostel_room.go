package domain

type Room struct {
	RoomID        string `json:"room_id"`
	BlockID       string `json:"block_id"`
	RoomNo        string `json:"room_no"`
	Capacity      int64  `json:"capacity"`
	OccupiedBeds  int64  `json:"occupied_beds"`
	AvailableBeds int64  `json:"available_beds"`
}
