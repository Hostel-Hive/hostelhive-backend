package provisioning

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
	"strings"
	"testing"
)

type memoryRepository struct {
	job     *Reservation
	account *authentication.Account
	fail    string
}

func (r *memoryRepository) Reserve(_ context.Context, v Reservation) error {
	if r.fail == "reserve" {
		return ErrUnavailable
	}
	if r.job != nil {
		if r.job.Key != v.Key || r.job.RequesterUID != v.RequesterUID || r.job.Email != v.Email || r.job.Role != v.Role {
			return ErrConflict
		}
		return nil
	}
	r.job = &v
	return nil
}
func (r *memoryRepository) Begin(context.Context, string) (Work, error) {
	if r.fail == "begin" {
		return nil, ErrInProgress
	}
	return &memoryWork{repository: r}, nil
}

type memoryWork struct {
	repository     *memoryRepository
	pendingAccount authentication.Account
	closed         bool
}

func (w *memoryWork) Reservation() Reservation { return *w.repository.job }
func (w *memoryWork) Account(context.Context) (authentication.Account, error) {
	if w.repository.fail == "account" {
		return authentication.Account{}, ErrUnavailable
	}
	return *w.repository.account, nil
}
func (w *memoryWork) InsertInactive(context.Context) (authentication.Account, error) {
	if w.repository.fail == "insert" {
		return authentication.Account{}, ErrUnavailable
	}
	r := w.repository.job
	w.pendingAccount = authentication.Account{UserID: "local-id", FirebaseUID: r.FirebaseUID, Email: r.Email, Role: r.Role}
	return w.pendingAccount, nil
}
func (w *memoryWork) Complete(context.Context, string) error {
	if w.repository.fail == "complete" {
		return ErrUnavailable
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
		return ErrUnavailable
	}
	if _, ok := i.users[uid]; !ok {
		i.users[uid] = false
		i.emails[uid] = email
		i.creates++
	}
	if i.fail == "create-after" {
		return ErrUnavailable
	}
	return nil
}
func (i *memoryIdentities) Enable(_ context.Context, uid, email string) error {
	if i.fail == "enable-before" {
		return ErrUnavailable
	}
	i.users[uid] = true
	i.enables++
	if i.fail == "enable-after" {
		return ErrUnavailable
	}
	return nil
}
func validInput() Input {
	return Input{Email: "new.user@example.invalid", Role: authentication.RoleStudent, Password: "Local-test-password-20"}
}
func TestProvisioningSuccessReplayAndConflict(t *testing.T) {
	r := &memoryRepository{}
	i := newMemoryIdentities()
	s := NewService(r, i)
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
	r.account.Role = authentication.RoleWarden
	r.account.IsActive = false
	retry, err = s.Provision(ctx, "request20", input, "admin-uid")
	if err != nil || retry.Account.IsActive || retry.Account.Role != authentication.RoleWarden {
		t.Fatalf("replay overwrote later changes: %+v %v", retry, err)
	}
	for _, v := range []struct {
		key, actor string
		input      Input
	}{{"request20", "other-admin", input}, {"newrequest20", "admin-uid", input}, {"request20", "admin-uid", Input{Email: input.Email, Role: authentication.RoleAdmin, Password: input.Password}}} {
		if _, err := s.Provision(ctx, v.key, v.input, v.actor); !errors.Is(err, ErrConflict) {
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
			s := NewService(r, i)
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
	for _, change := range []func(*Input){func(v *Input) { v.Email = "invalid" }, func(v *Input) { v.Email = "Name <test@example.invalid>" }, func(v *Input) { v.Role = "superuser" }, func(v *Input) { v.Role = "Student" }, func(v *Input) { v.Password = "short" }, func(v *Input) { v.Password = string([]byte{0xff}) }} {
		v := validInput()
		change(&v)
		r := &memoryRepository{}
		if _, err := NewService(r, newMemoryIdentities()).Provision(context.Background(), "request20", v, "admin"); !errors.Is(err, ErrInvalidInput) || r.job != nil {
			t.Errorf("invalid input persisted: %v", err)
		}
	}
	v := validInput()
	v.Email = "  NEW.User@Example.Invalid  "
	normalized, err := Normalize(v)
	if err != nil || normalized.Email != "new.user@example.invalid" {
		t.Fatal("email normalization failed")
	}
	for _, key := range []string{"", "short", "space key", "slash/key"} {
		if _, err := NewService(&memoryRepository{}, newMemoryIdentities()).Provision(context.Background(), key, validInput(), "admin"); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("accepted key %q", key)
		}
	}
}
func TestProvisioningInputBoundaries(t *testing.T) {
	for _, role := range []string{authentication.RoleAdmin, authentication.RoleWarden, authentication.RoleSubWarden, authentication.RoleSecurityStaff, authentication.RoleStudent} {
		for _, length := range []int{12, 128} {
			v := validInput()
			v.Role = role
			v.Password = strings.Repeat("\u754c", length)
			if _, err := Normalize(v); err != nil {
				t.Errorf("rejected role or character boundary: %v", err)
			}
		}
	}
	for _, length := range []int{11, 129} {
		v := validInput()
		v.Password = strings.Repeat("x", length)
		if _, err := Normalize(v); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("accepted password length %d", length)
		}
	}
	v := validInput()
	v.Email = strings.Repeat("x", 250) + "@example.invalid"
	if _, err := Normalize(v); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("accepted oversized email")
	}
}
