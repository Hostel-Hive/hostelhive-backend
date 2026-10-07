package domain

import (
	"errors"
)

var (
	ErrProvisionInvalidInput = errors.New("invalid provisioning input")
	ErrProvisionConflict     = errors.New("provisioning conflict")
	ErrProvisionUnavailable  = errors.New("provisioning unavailable; retry with the same request key")
	ErrProvisionInProgress   = errors.New("provisioning request in progress")
)

type Reservation struct {
	Key, RequesterUID, FirebaseUID, Email, Role string
	Completed                                   bool
}
