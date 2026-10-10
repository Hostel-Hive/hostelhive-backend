package dto

type Assign struct {
	StudentID string `json:"student_id"`
	BedID     string `json:"bed_id"`
}
type Transfer struct {
	BedID string `json:"bed_id"`
}
