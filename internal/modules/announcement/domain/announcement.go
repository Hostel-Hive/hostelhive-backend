package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid announcement input")
	ErrForbidden   = errors.New("announcement forbidden")
	ErrNotFound    = errors.New("announcement not found")
	ErrConflict    = errors.New("announcement request reused")
	ErrUnavailable = errors.New("announcements unavailable")
)

type Announcement struct {
	AnnouncementID string    `json:"announcement_id"`
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	Audience       string    `json:"audience"`
	Priority       string    `json:"priority"`
	PublishedBy    string    `json:"published_by"`
	PublishedAt    time.Time `json:"published_at"`
}
type Publication struct {
	Announcement Announcement `json:"announcement"`
	Replayed     bool         `json:"replayed"`
}
type Page struct {
	Items   []Announcement `json:"items"`
	Total   int64          `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	HasMore bool           `json:"has_more"`
}
type DeliverySummary struct {
	AnnouncementID  string `json:"announcement_id"`
	RecipientsTotal int64  `json:"recipients_total"`
	Delivered       int64  `json:"delivered"`
	PendingDelivery int64  `json:"pending_delivery"`
	Read            int64  `json:"read"`
	Unread          int64  `json:"unread"`
}
