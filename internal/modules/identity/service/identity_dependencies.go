package service

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
)

type ActivationRepository interface {
	BeginActivation(context.Context, string, string) (ActivationWork, error)
}

type ActivationWork interface {
	Account() domain.Account
	Complete(context.Context) (domain.Account, error)
	Close()
}

type Reactivator interface {
	Reactivate(context.Context, string, string) error
}

type Identities interface {
	EnsureDisabled(context.Context, string, string, string) error
	Enable(context.Context, string, string) error
}

type Repository interface {
	Reserve(context.Context, domain.Reservation) error
	Begin(context.Context, string) (Work, error)
}

type Work interface {
	Reservation() domain.Reservation
	Account(context.Context) (domain.Account, error)
	InsertInactive(context.Context) (domain.Account, error)
	Complete(context.Context, string) error
	Close()
}

type Revoker interface {
	DisableAndRevoke(context.Context, string) error
}
type AccountRepository interface {
	List(context.Context, int, int) (domain.AccountPage, error)
	Change(context.Context, string, string, string, bool) (domain.AccountChange, error)
	RetryOne(context.Context, Revoker, string) (bool, error)
	Pending(context.Context, string) (bool, error)
}
