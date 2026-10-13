package domain

import "time"

// Projection contains no identity, contact details, QR secrets or event history.
// Version is also an invalidation signal: clients refetch authorized API lists.
type Projection struct {
	Version     int64           `json:"version"`
	ChangedAt   int64           `json:"changed_at"`
	PublishedAt int64           `json:"published_at"`
	Summary     PresenceSummary `json:"summary"`
}

type Grant struct {
	Version   int64 `json:"version"`
	ExpiresAt int64 `json:"expires_at"`
}

type RealtimeSession struct {
	DatabaseURL       string    `json:"database_url"`
	Path              string    `json:"path"`
	ExpiresAt         time.Time `json:"expires_at"`
	RenewAfterSeconds int       `json:"renew_after_seconds"`
}

type PublicationStatus struct {
	Enabled          bool       `json:"enabled"`
	Pending          bool       `json:"pending"`
	Version          int64      `json:"version"`
	PublishedVersion *int64     `json:"published_version"`
	ChangedAt        time.Time  `json:"changed_at"`
	PublishedAt      *time.Time `json:"published_at"`
	Attempts         int        `json:"attempts"`
	PendingGrants    int64      `json:"pending_grants"`
}
