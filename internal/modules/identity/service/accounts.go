package service

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
)

type Accounts struct {
	store   AccountRepository
	revoker Revoker
}

func NewAccounts(store AccountRepository, revoker Revoker) *Accounts {
	return &Accounts{store, revoker}
}
func (s *Accounts) List(ctx context.Context, limit, offset int) (domain.AccountPage, error) {
	return s.store.List(ctx, limit, offset)
}
func (s *Accounts) ChangeRole(ctx context.Context, actor, id, role string) (domain.Account, error) {
	result, err := s.store.Change(ctx, actor, id, role, false)
	return result.Account, err
}
func (s *Accounts) Deactivate(ctx context.Context, actor, id string) (domain.AccountChange, error) {
	result, err := s.store.Change(ctx, actor, id, "", true)
	if err != nil {
		return result, err
	}
	// Local denial and the durable job commit precede external side effects.
	_, _ = s.store.RetryOne(ctx, s.revoker, id)
	pending, err := s.store.Pending(ctx, id)
	if err != nil {
		pending = true
	}
	result.RevocationPending = pending
	return result, nil
}
