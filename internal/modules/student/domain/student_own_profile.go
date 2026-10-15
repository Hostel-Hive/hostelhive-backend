package domain

import "time"

type OwnProfile struct {
	StudentID       string            `json:"student_id"`
	IndexNo         string            `json:"index_no"`
	FullName        string            `json:"full_name"`
	Faculty         string            `json:"faculty"`
	Year            int               `json:"year"`
	ContactPhone    string            `json:"contact_phone"`
	Email           string            `json:"email"`
	Guardians       []Guardian        `json:"guardians"`
	ProfileImageURL *string           `json:"profile_image_url"`
	Accommodation   *OwnAccommodation `json:"accommodation"`
	Presence        OwnPresence       `json:"presence"`
}
type OwnAccommodation struct {
	AllocationID string    `json:"allocation_id"`
	BlockID      string    `json:"block_id"`
	BlockName    string    `json:"block_name"`
	RoomID       string    `json:"room_id"`
	RoomNo       string    `json:"room_no"`
	BedID        string    `json:"bed_id"`
	BedNo        string    `json:"bed_no"`
	StartedAt    time.Time `json:"started_at"`
}
type OwnPresence struct {
	Status    string     `json:"status"`
	UpdatedAt *time.Time `json:"updated_at"`
}
