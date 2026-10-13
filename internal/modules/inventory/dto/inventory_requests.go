package dto

type BlockDetails struct {
	Name string `json:"name"`
}

type CreateRoom struct {
	BlockID string `json:"block_id"`
	RoomNo  string `json:"room_no"`
}

type RoomDetails struct {
	RoomNo string `json:"room_no"`
}

type CreateBed struct {
	RoomID string `json:"room_id"`
	BedNo  string `json:"bed_no"`
}

type BedDetails struct {
	BedNo string `json:"bed_no"`
}
