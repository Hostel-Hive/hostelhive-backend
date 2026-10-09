package provisioning_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	identityservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
)

type failCompleteRepository struct{ identityservice.Repository }

func (r failCompleteRepository) Begin(ctx context.Context, key string) (identityservice.Work, error) {
	w, err := r.Repository.Begin(ctx, key)
	if err != nil {
		return nil, err
	}
	return failCompleteWork{w}, nil
}

type failCompleteWork struct{ identityservice.Work }

func (w failCompleteWork) Complete(context.Context, string) error {
	w.Work.Close()
	return domain.ErrProvisionUnavailable
}

func TestProvisioningPostgresIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable database migrated to version 3")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal("test database connection failed")
	}
	defer pool.Close()
	prefix := fmt.Sprintf("issue20-%d", time.Now().UnixNano())
	// This workflow commits reservations and completion, so remove only the
	// fixtures belonging to this unique test prefix, in FK-safe order.
	defer func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanCtx, `DELETE FROM hostelhive.user_provisioning WHERE request_key LIKE $1`, prefix+"%"); err != nil {
			t.Error("provisioning fixture cleanup failed")
		}
		if _, err := pool.Exec(cleanCtx, `DELETE FROM hostelhive.users WHERE email LIKE $1`, prefix+"%"); err != nil {
			t.Error("user fixture cleanup failed")
		}
	}()
	repository := identityrepo.NewPostgresRepository(pool)
	identities := newMemoryIdentities()
	service := identityservice.NewProvisioningService(repository, identities)
	input := validInput()
	input.Email = prefix + "-success@example.invalid"
	key := prefix + "-success"
	first, err := service.Provision(ctx, key, input, "admin-uid")
	if err != nil || !first.Account.IsActive {
		t.Fatalf("first provision: %+v %v", first, err)
	}
	second, err := identityservice.NewProvisioningService(identityrepo.NewPostgresRepository(pool), identities).Provision(ctx, key, input, "admin-uid")
	if err != nil || !second.Replayed || second.Account.UserID != first.Account.UserID || identities.creates != 1 || identities.enables != 1 {
		t.Fatal("durable replay failed")
	}
	if _, err = pool.Exec(ctx, `UPDATE hostelhive.users SET role='warden',is_active=false WHERE user_id=$1`, first.Account.UserID); err != nil {
		t.Fatal(err)
	}
	replay, err := service.Provision(ctx, key, input, "admin-uid")
	if err != nil || replay.Account.IsActive || replay.Account.Role != domain.RoleWarden {
		t.Fatal("replay reactivated or overwrote account")
	}
	changed := input
	changed.Role = domain.RoleAdmin
	for _, tc := range []struct {
		key, actor string
		input      dto.Input
	}{{key, "admin-uid", changed}, {key, "other-admin", input}, {prefix + "-duplicate", "admin-uid", input}} {
		if _, err := service.Provision(ctx, tc.key, tc.input, tc.actor); !errors.Is(err, domain.ErrProvisionConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	}
	for _, stage := range []string{"create-after", "enable-after", "complete"} {
		t.Run(stage, func(t *testing.T) {
			input := validInput()
			input.Email = prefix + "-" + stage + "@example.invalid"
			key := prefix + "-" + stage
			identities := newMemoryIdentities()
			var selected identityservice.Repository = repository
			if stage == "complete" {
				selected = failCompleteRepository{repository}
			} else {
				identities.fail = stage
			}
			if _, err := identityservice.NewProvisioningService(selected, identities).Provision(ctx, key, input, "admin-uid"); err == nil {
				t.Fatal("failure reported success")
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM hostelhive.users WHERE email=$1 AND is_active`, input.Email).Scan(&count); err != nil || count != 0 {
				t.Fatal("partial failure granted access")
			}
			var uid string
			if err := pool.QueryRow(ctx, `SELECT firebase_uid FROM hostelhive.user_provisioning WHERE request_key=$1 AND completed_at IS NULL`, key).Scan(&uid); err != nil {
				t.Fatal("reservation was lost")
			}
			identities.fail = ""
			result, err := identityservice.NewProvisioningService(repository, identities).Provision(ctx, key, input, "admin-uid")
			if err != nil || !result.Account.IsActive || result.Account.FirebaseUID != uid || identities.creates != 1 {
				t.Fatalf("recovery failed: %+v %v", result, err)
			}
		})
	}
	lockKey := prefix + "-lock"
	if err := repository.Reserve(ctx, domain.Reservation{Key: lockKey, RequesterUID: "admin-uid", FirebaseUID: prefix + "-uid-lock", Email: prefix + "-lock@example.invalid", Role: domain.RoleStudent}); err != nil {
		t.Fatal(err)
	}
	work, err := repository.Begin(ctx, lockKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Begin(ctx, lockKey); !errors.Is(err, domain.ErrProvisionInProgress) {
		work.Close()
		t.Fatalf("concurrent worker not rejected: %v", err)
	}
	work.Close()
	next, err := repository.Begin(ctx, lockKey)
	if err != nil {
		t.Fatal("worker lock was not released")
	}
	next.Close()
	var secrets int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='hostelhive' AND table_name='user_provisioning' AND (column_name LIKE '%password%' OR column_name LIKE '%token%')`).Scan(&secrets)
	if err != nil || secrets != 0 {
		t.Fatal("credential material in provisioning schema")
	}
}

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
