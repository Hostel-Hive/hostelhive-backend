package domain

import "errors"

var (
	ErrRevocationPending      = errors.New("revocation pending")
	ErrProvisioningIncomplete = errors.New("provisioning incomplete")
	ErrIdentityMismatch       = errors.New("firebase identity mismatch")
	ErrIdentityMissing        = errors.New("firebase identity missing")
)
