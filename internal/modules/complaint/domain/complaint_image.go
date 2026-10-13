package domain

import "time"

const MaxComplaintImageBytes = 5 * 1024 * 1024
const MaxComplaintImages = 3

type Image struct {
	ImageID         string     `json:"image_id"`
	ComplaintID     string     `json:"complaint_id"`
	ContentType     string     `json:"content_type"`
	Size            int        `json:"size_bytes"`
	Width           int        `json:"width"`
	Height          int        `json:"height"`
	UploadedAt      *time.Time `json:"uploaded_at"`
	Key             string     `json:"-"`
	Hash            []byte     `json:"-"`
	State           string     `json:"-"`
	AttachedVersion *int64     `json:"-"`
	RemovedVersion  *int64     `json:"-"`
}
type ImageResult struct {
	Image    Image `json:"image"`
	Version  int64 `json:"version"`
	Replayed bool  `json:"replayed"`
}
type ImageList struct {
	Items   []Image `json:"items"`
	Version int64   `json:"version"`
}
type ImageRemoval struct {
	ImageID  string `json:"image_id"`
	Removed  bool   `json:"removed"`
	Version  int64  `json:"version"`
	Replayed bool   `json:"replayed"`
}
