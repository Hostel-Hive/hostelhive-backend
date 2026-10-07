package domain

import (
	"errors"
)

var (
	ErrManagementNotFound    = errors.New("account not found")
	ErrManagementForbidden   = errors.New("forbidden")
	ErrLastAdmin             = errors.New("last active administrator")
	ErrManagementUnavailable = errors.New("account management unavailable")
)

type AccountPage struct {
	Users   []Account `json:"users"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"has_more"`
}

type AccountChange struct {
	Account           Account `json:"account"`
	RevocationPending bool    `json:"revocation_pending"`
}
