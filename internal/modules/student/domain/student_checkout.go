package domain

import (
	"errors"
	"time"
)

var (
	ErrCheckoutConflict   = errors.New("checkout request conflicts with a previous operation")
	ErrAlreadyCheckedOut  = errors.New("student stay already ended")
	ErrCheckoutIneligible = errors.New("linked account must have student role")
)

// Checkout is the immutable departure snapshot, not the student's current state
// after subsequent restoration, reactivation or accommodation changes.
type Checkout struct {
	CheckoutID           string    `json:"checkout_id"`
	RequestID            string    `json:"request_id"`
	StudentID            string    `json:"student_id"`
	UserID               string    `json:"user_id"`
	Reason               string    `json:"reason"`
	Notes                string    `json:"notes"`
	DeactivateAccount    bool      `json:"deactivate_account"`
	AccountActiveAfter   bool      `json:"account_active_after"`
	ProfileWasArchived   bool      `json:"profile_was_archived"`
	ArchivedAt           time.Time `json:"archived_at"`
	ReleasedAllocationID *string   `json:"released_allocation_id"`
	ReleasedBedID        *string   `json:"released_bed_id"`
	PerformedBy          string    `json:"performed_by"`
	PerformedAt          time.Time `json:"performed_at"`
}
type CheckoutResult struct {
	Checkout          Checkout `json:"checkout"`
	Replayed          bool     `json:"replayed"`
	RevocationPending bool     `json:"revocation_pending"`
}
