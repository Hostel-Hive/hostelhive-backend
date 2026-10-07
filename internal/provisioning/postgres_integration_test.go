package provisioning

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
	"github.com/jackc/pgx/v5/pgxpool"
)

type failCompleteRepository struct{ Repository }

func (r failCompleteRepository) Begin(ctx context.Context, key string) (Work, error) {
	w, err := r.Repository.Begin(ctx, key)
	if err != nil {
		return nil, err
	}
	return failCompleteWork{w}, nil
}

type failCompleteWork struct{ Work }

func (w failCompleteWork) Complete(context.Context, string) error {
	w.Work.Close()
	return ErrUnavailable
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
	repository := NewPostgresRepository(pool)
	identities := newMemoryIdentities()
	service := NewService(repository, identities)
	input := validInput()
	input.Email = prefix + "-success@example.invalid"
	key := prefix + "-success"
	first, err := service.Provision(ctx, key, input, "admin-uid")
	if err != nil || !first.Account.IsActive {
		t.Fatalf("first provision: %+v %v", first, err)
	}
	second, err := NewService(NewPostgresRepository(pool), identities).Provision(ctx, key, input, "admin-uid")
	if err != nil || !second.Replayed || second.Account.UserID != first.Account.UserID || identities.creates != 1 || identities.enables != 1 {
		t.Fatal("durable replay failed")
	}
	if _, err = pool.Exec(ctx, `UPDATE hostelhive.users SET role='warden',is_active=false WHERE user_id=$1`, first.Account.UserID); err != nil {
		t.Fatal(err)
	}
	replay, err := service.Provision(ctx, key, input, "admin-uid")
	if err != nil || replay.Account.IsActive || replay.Account.Role != authentication.RoleWarden {
		t.Fatal("replay reactivated or overwrote account")
	}
	changed := input
	changed.Role = authentication.RoleAdmin
	for _, tc := range []struct {
		key, actor string
		input      Input
	}{{key, "admin-uid", changed}, {key, "other-admin", input}, {prefix + "-duplicate", "admin-uid", input}} {
		if _, err := service.Provision(ctx, tc.key, tc.input, tc.actor); !errors.Is(err, ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	}
	for _, stage := range []string{"create-after", "enable-after", "complete"} {
		t.Run(stage, func(t *testing.T) {
			input := validInput()
			input.Email = prefix + "-" + stage + "@example.invalid"
			key := prefix + "-" + stage
			identities := newMemoryIdentities()
			var selected Repository = repository
			if stage == "complete" {
				selected = failCompleteRepository{repository}
			} else {
				identities.fail = stage
			}
			if _, err := NewService(selected, identities).Provision(ctx, key, input, "admin-uid"); err == nil {
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
			result, err := NewService(repository, identities).Provision(ctx, key, input, "admin-uid")
			if err != nil || !result.Account.IsActive || result.Account.FirebaseUID != uid || identities.creates != 1 {
				t.Fatalf("recovery failed: %+v %v", result, err)
			}
		})
	}
	lockKey := prefix + "-lock"
	if err := repository.Reserve(ctx, Reservation{Key: lockKey, RequesterUID: "admin-uid", FirebaseUID: prefix + "-uid-lock", Email: prefix + "-lock@example.invalid", Role: authentication.RoleStudent}); err != nil {
		t.Fatal(err)
	}
	work, err := repository.Begin(ctx, lockKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Begin(ctx, lockKey); !errors.Is(err, ErrInProgress) {
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
