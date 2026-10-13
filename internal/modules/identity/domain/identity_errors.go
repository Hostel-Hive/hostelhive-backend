package domain

import "errors"

var (
	ErrAccountNotFound        = errors.New("account not found")
	ErrManagementNotFound     = errors.New("account not found")
	ErrManagementForbidden    = errors.New("forbidden")
	ErrLastAdmin              = errors.New("last active administrator")
	ErrManagementUnavailable  = errors.New("account management unavailable")
	ErrProvisionInvalidInput  = errors.New("invalid provisioning input")
	ErrProvisionConflict      = errors.New("provisioning conflict")
	ErrProvisionUnavailable   = errors.New("provisioning unavailable; retry with the same request key")
	ErrProvisionInProgress    = errors.New("provisioning request in progress")
	ErrRevocationPending      = errors.New("revocation pending")
	ErrProvisioningIncomplete = errors.New("provisioning incomplete")
	ErrIdentityMismatch       = errors.New("firebase identity mismatch")
	ErrIdentityMissing        = errors.New("firebase identity missing")
)
