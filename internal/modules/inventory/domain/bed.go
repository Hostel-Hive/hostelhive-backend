package domain

type Bed struct {
	BedID     string `json:"bed_id"`
	RoomID    string `json:"room_id"`
	BlockID   string `json:"block_id"`
	BedNo     string `json:"bed_no"`
	Available bool   `json:"available"`
}
