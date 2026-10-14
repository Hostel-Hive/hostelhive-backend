package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
)

type memoryRepository struct {
	job     *domain.Reservation
	account *domain.Account
	fail    string
}

func (r *memoryRepository) Reserve(_ context.Context, v domain.Reservation) error {
	if r.fail == "reserve" {
		return domain.ErrProvisionUnavailable
	}
	if r.job != nil {
		if r.job.Key != v.Key || r.job.RequesterUID != v.RequesterUID || r.job.Email != v.Email || r.job.Role != v.Role {
			return domain.ErrProvisionConflict
		}
		return nil
	}
	r.job = &v
	return nil
}

func (r *memoryRepository) Begin(context.Context, string) (Work, error) {
	if r.fail == "begin" {
		return nil, domain.ErrProvisionInProgress
	}
	return &memoryWork{repository: r}, nil
}

type memoryWork struct {
	repository     *memoryRepository
	pendingAccount domain.Account
	closed         bool
}

func (w *memoryWork) Reservation() domain.Reservation { return *w.repository.job }

func (w *memoryWork) Account(context.Context) (domain.Account, error) {
	if w.repository.fail == "account" {
		return domain.Account{}, domain.ErrProvisionUnavailable
	}
	return *w.repository.account, nil
}

func (w *memoryWork) InsertInactive(context.Context) (domain.Account, error) {
	if w.repository.fail == "insert" {
		return domain.Account{}, domain.ErrProvisionUnavailable
	}
	r := w.repository.job
	w.pendingAccount = domain.Account{UserID: "local-id", FirebaseUID: r.FirebaseUID, Email: r.Email, Role: r.Role}
	return w.pendingAccount, nil
}

func (w *memoryWork) Complete(context.Context, string) error {
	if w.repository.fail == "complete" {
		return domain.ErrProvisionUnavailable
	}
	a := w.pendingAccount
	a.IsActive = true
	w.repository.account = &a
	w.repository.job.Completed = true
	return nil
}

func (w *memoryWork) Close() { w.closed = true }

type memoryIdentities struct {
	users            map[string]bool
	emails           map[string]string
	creates, enables int
	fail             string
}

func newMemoryIdentities() *memoryIdentities {
	return &memoryIdentities{users: map[string]bool{}, emails: map[string]string{}}
}

func (i *memoryIdentities) EnsureDisabled(_ context.Context, uid, email, password string) error {
	if i.fail == "create-before" {
		return domain.ErrProvisionUnavailable
	}
	if _, ok := i.users[uid]; !ok {
		i.users[uid] = false
		i.emails[uid] = email
		i.creates++
	}
	if i.fail == "create-after" {
		return domain.ErrProvisionUnavailable
	}
	return nil
}

func (i *memoryIdentities) Enable(_ context.Context, uid, email string) error {
	if i.fail == "enable-before" {
		return domain.ErrProvisionUnavailable
	}
	i.users[uid] = true
	i.enables++
	if i.fail == "enable-after" {
		return domain.ErrProvisionUnavailable
	}
	return nil
}

func validInput() dto.Input {
	return dto.Input{Email: "new.user@example.invalid", Role: domain.RoleStudent, Password: "Local-test-password-20"}
}

func TestProvisioningSuccessReplayAndConflict(t *testing.T) {
	r := &memoryRepository{}
	i := newMemoryIdentities()
	s := NewProvisioningService(r, i)
	ctx := context.Background()
	input := validInput()
	first, err := s.Provision(ctx, "request20", input, "admin-uid")
	if err != nil || !first.Account.IsActive || first.Replayed || i.creates != 1 {
		t.Fatalf("initial result: %+v %v", first, err)
	}
	input.Password = "different-password-20"
	retry, err := s.Provision(ctx, "request20", input, "admin-uid")
	if err != nil || !retry.Replayed || retry.Account.UserID != first.Account.UserID || i.creates != 1 || i.enables != 1 {
		t.Fatalf("replay: %+v %v", retry, err)
	}
	r.account.Role = domain.RoleWarden
	r.account.IsActive = false
	retry, err = s.Provision(ctx, "request20", input, "admin-uid")
	if err != nil || retry.Account.IsActive || retry.Account.Role != domain.RoleWarden {
		t.Fatalf("replay overwrote later changes: %+v %v", retry, err)
	}
	for _, v := range []struct {
		key, actor string
		input      dto.Input
	}{{"request20", "other-admin", input}, {"newrequest20", "admin-uid", input}, {"request20", "admin-uid", dto.Input{Email: input.Email, Role: domain.RoleAdmin, Password: input.Password}}} {
		if _, err := s.Provision(ctx, v.key, v.input, v.actor); !errors.Is(err, domain.ErrProvisionConflict) {
			t.Errorf("expected conflict: %v", err)
		}
	}
}

func TestPartialFailureSafeRecovery(t *testing.T) {
	for _, failure := range []string{"reserve", "begin", "insert", "create-before", "create-after", "enable-before", "enable-after", "complete"} {
		t.Run(failure, func(t *testing.T) {
			r := &memoryRepository{fail: failure}
			i := newMemoryIdentities()
			i.fail = failure
			s := NewProvisioningService(r, i)
			if _, err := s.Provision(context.Background(), "request20", validInput(), "admin-uid"); err == nil {
				t.Fatal("expected failure")
			}
			if r.account != nil && r.account.IsActive {
				t.Fatal("partial failure granted access")
			}
			uid := ""
			if r.job != nil {
				uid = r.job.FirebaseUID
				if r.job.Completed {
					t.Fatal("partial failure completed reservation")
				}
			}
			r.fail = ""
			i.fail = ""
			result, err := s.Provision(context.Background(), "request20", validInput(), "admin-uid")
			if err != nil || !result.Account.IsActive || i.creates != 1 {
				t.Fatalf("recovery: %+v %v creates=%d", result, err, i.creates)
			}
			if uid != "" && result.Account.FirebaseUID != uid {
				t.Fatal("recovery changed reserved UID")
			}
		})
	}
}

func TestValidateProvisioningInput(t *testing.T) {
	for _, change := range []func(*dto.Input){func(v *dto.Input) { v.Email = "invalid" }, func(v *dto.Input) { v.Email = "Name <test@example.invalid>" }, func(v *dto.Input) { v.Role = "superuser" }, func(v *dto.Input) { v.Role = "Student" }, func(v *dto.Input) { v.Password = "short" }, func(v *dto.Input) { v.Password = string([]byte{0xff}) }} {
		v := validInput()
		change(&v)
		r := &memoryRepository{}
		if _, err := NewProvisioningService(r, newMemoryIdentities()).Provision(context.Background(), "request20", v, "admin"); !errors.Is(err, domain.ErrProvisionInvalidInput) || r.job != nil {
			t.Errorf("invalid input persisted: %v", err)
		}
	}
	v := validInput()
	v.Email = "  NEW.User@Example.Invalid  "
	normalized, err := NormalizeProvisioning(v)
	if err != nil || normalized.Email != "new.user@example.invalid" {
		t.Fatal("email normalization failed")
	}
	for _, key := range []string{"", "short", "space key", "slash/key"} {
		if _, err := NewProvisioningService(&memoryRepository{}, newMemoryIdentities()).Provision(context.Background(), key, validInput(), "admin"); !errors.Is(err, domain.ErrProvisionInvalidInput) {
			t.Errorf("accepted key %q", key)
		}
	}
}

func TestProvisioningInputBoundaries(t *testing.T) {
	for _, role := range []string{domain.RoleAdmin, domain.RoleWarden, domain.RoleSubWarden, domain.RoleSecurityStaff, domain.RoleStudent} {
		for _, length := range []int{12, 128} {
			v := validInput()
			v.Role = role
			v.Password = strings.Repeat("\u754c", length)
			if _, err := NormalizeProvisioning(v); err != nil {
				t.Errorf("rejected role or character boundary: %v", err)
			}
		}
	}
	for _, length := range []int{11, 129} {
		v := validInput()
		v.Password = strings.Repeat("x", length)
		if _, err := NormalizeProvisioning(v); !errors.Is(err, domain.ErrProvisionInvalidInput) {
			t.Errorf("accepted password length %d", length)
		}
	}
	v := validInput()
	v.Email = strings.Repeat("x", 250) + "@example.invalid"
	if _, err := NormalizeProvisioning(v); !errors.Is(err, domain.ErrProvisionInvalidInput) {
		t.Fatal("accepted oversized email")
	}
}
