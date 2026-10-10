package domain

type Block struct {
	BlockID       string `json:"block_id"`
	Name          string `json:"name"`
	RoomCount     int64  `json:"room_count"`
	Capacity      int64  `json:"capacity"`
	OccupiedBeds  int64  `json:"occupied_beds"`
	AvailableBeds int64  `json:"available_beds"`
}
