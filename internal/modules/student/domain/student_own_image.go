package domain

// OwnImageResult never exposes the storage key or a staff-only download URL.
type OwnImageResult struct {
	StudentID       string `json:"student_id"`
	ProfileImageURL string `json:"profile_image_url"`
}
