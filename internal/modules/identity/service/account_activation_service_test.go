package service

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"testing"
)

type activationFixture struct {
	active, closed, committed        bool
	beginErr, providerErr, commitErr error
	calls                            int
}

func (f *activationFixture) BeginActivation(context.Context, string, string) (ActivationWork, error) {
	return f, f.beginErr
}
func (f *activationFixture) Account() domain.Account {
	return domain.Account{FirebaseUID: "uid", Email: "user@example.invalid", IsActive: f.active}
}
func (f *activationFixture) Complete(context.Context) (domain.Account, error) {
	f.committed = true
	return domain.Account{IsActive: f.commitErr == nil}, f.commitErr
}
func (f *activationFixture) Close() { f.closed = true }
func (f *activationFixture) Reactivate(context.Context, string, string) error {
	f.calls++
	if f.committed {
		panic("activated locally before Firebase")
	}
	return f.providerErr
}
func TestActivationFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name          string
		f             activationFixture
		want          error
		calls         int
		commit, close bool
	}{
		{"success", activationFixture{}, nil, 1, true, true},
		{"already active", activationFixture{active: true}, nil, 0, false, true},
		{"pending", activationFixture{beginErr: domain.ErrRevocationPending}, domain.ErrRevocationPending, 0, false, false},
		{"Firebase failure", activationFixture{providerErr: domain.ErrManagementUnavailable}, domain.ErrManagementUnavailable, 1, false, true},
		{"commit failure", activationFixture{commitErr: domain.ErrManagementUnavailable}, domain.ErrManagementUnavailable, 1, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			_, err := NewActivation(&f, &f).Activate(context.Background(), "admin", "id")
			if !errors.Is(err, tc.want) || f.calls != tc.calls || f.committed != tc.commit || f.closed != tc.close {
				t.Fatalf("unsafe activation: %+v %v", f, err)
			}
		})
	}
}
