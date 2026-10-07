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
type Activation struct {
	store    ActivationRepository
	provider Reactivator
}

func NewActivation(s ActivationRepository, p Reactivator) *Activation { return &Activation{s, p} }
func (s *Activation) Activate(ctx context.Context, actor, id string) (domain.Account, error) {
	w, err := s.store.BeginActivation(ctx, actor, id)
	if err != nil {
		return domain.Account{}, err
	}
	defer w.Close()
	a := w.Account()
	if a.IsActive {
		return a, nil
	}
	if err = s.provider.Reactivate(ctx, a.FirebaseUID, a.Email); err != nil {
		return domain.Account{}, err
	}
	return w.Complete(ctx)
}
