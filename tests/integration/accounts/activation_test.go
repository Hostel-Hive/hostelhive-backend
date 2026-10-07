package accounts_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type blockingRevocation struct{ entered, release chan struct{} }

func (b blockingRevocation) DisableAndRevoke(ctx context.Context, _ string) error {
	close(b.entered)
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type activationProvider struct {
	calls atomic.Int32
	fail  bool
}

func (p *activationProvider) Reactivate(context.Context, string, string) error {
	p.calls.Add(1)
	if p.fail {
		return domain.ErrManagementUnavailable
	}
	return nil
}
func TestPostgresActivationLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("activate-%d", time.Now().UnixNano())
	ids := []string{}
	uids := []string{}
	defer func() {
		pool.Exec(context.Background(), "DELETE FROM hostelhive.user_revocations WHERE user_id=ANY($1::uuid[])", ids)
		pool.Exec(context.Background(), "DELETE FROM hostelhive.user_provisioning WHERE firebase_uid=ANY($1::text[])", uids)
		pool.Exec(context.Background(), "DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])", ids)
	}()
	for n, role := range []string{"admin", "student"} {
		uid := fmt.Sprintf("%s-%d", prefix, n)
		a, e := scan(pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,$4) RETURNING "+columns, uid, uid+"@example.invalid", role, n == 0))
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, a.UserID)
		uids = append(uids, uid)
	}
	store := repository.NewStore(pool)
	provider := &activationProvider{fail: true}
	activation := service.NewActivation(store, provider)
	if _, err = activation.Activate(ctx, uids[1], ids[1]); !errors.Is(err, domain.ErrManagementForbidden) {
		t.Fatal("nonadmin allowed", err)
	}
	if _, err = activation.Activate(ctx, uids[0], "11111111-1111-1111-1111-111111111111"); !errors.Is(err, domain.ErrManagementNotFound) {
		t.Fatal("missing target", err)
	}
	if _, err = activation.Activate(ctx, uids[0], ids[1]); !errors.Is(err, domain.ErrManagementUnavailable) {
		t.Fatal(err)
	}
	a, e := scan(pool.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.users WHERE user_id=$1", ids[1]))
	if e != nil || a.IsActive {
		t.Fatal("provider failure activated local user")
	}
	if _, e = pool.Exec(ctx, "INSERT INTO hostelhive.user_revocations(user_id) VALUES($1)", ids[1]); e != nil {
		t.Fatal(e)
	}
	calls := provider.calls.Load()
	if _, err = activation.Activate(ctx, uids[0], ids[1]); !errors.Is(err, domain.ErrRevocationPending) || provider.calls.Load() != calls {
		t.Fatal("pending revocation bypassed", err)
	}
	if _, e = pool.Exec(ctx, "UPDATE hostelhive.user_revocations SET completed_at=clock_timestamp() WHERE user_id=$1", ids[1]); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, "INSERT INTO hostelhive.user_provisioning(request_key,requester_uid,firebase_uid,email,role) VALUES($1,$2,$3,$4,'student')", prefix, uids[0], uids[1], uids[1]+"@example.invalid"); e != nil {
		t.Fatal(e)
	}
	if _, err = activation.Activate(ctx, uids[0], ids[1]); !errors.Is(err, domain.ErrProvisioningIncomplete) {
		t.Fatal("incomplete provisioning bypassed", err)
	}
	pool.Exec(ctx, "DELETE FROM hostelhive.user_provisioning WHERE request_key=$1", prefix)
	provider.fail = false
	calls = provider.calls.Load()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, e := activation.Activate(ctx, uids[0], ids[1])
			if e == nil && (!a.IsActive || a.Role != "student") {
				e = errors.New("wrong account state")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if provider.calls.Load() != calls+1 {
		t.Fatal("concurrent activation was not idempotent")
	}
	if _, err = store.Change(ctx, uids[0], ids[1], "", true); err != nil {
		t.Fatal(err)
	}
	if _, err = activation.Activate(ctx, uids[0], ids[1]); !errors.Is(err, domain.ErrRevocationPending) {
		t.Fatal("new deactivation bypassed", err)
	}
	// A worker already holding the job lock must finish before activation can
	// enable Firebase. A bounded request times out safely while the worker runs.
	blocker := blockingRevocation{make(chan struct{}), make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, e := store.RetryOne(ctx, blocker, ids[1]); done <- e }()
	select {
	case <-blocker.entered:
	case <-ctx.Done():
		t.Fatal("worker never acquired job")
	}
	short, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	calls = provider.calls.Load()
	_, err = activation.Activate(short, uids[0], ids[1])
	stop()
	close(blocker.release)
	if !errors.Is(err, domain.ErrManagementUnavailable) || provider.calls.Load() != calls {
		t.Fatal("activation bypassed in-flight worker", err)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if _, err = activation.Activate(ctx, uids[0], ids[1]); err != nil {
		t.Fatal("activation after worker", err)
	}
	if worked, e := store.RetryOne(ctx, blocker, ids[1]); e != nil || worked {
		t.Fatal("completed job retried after activation", e)
	}

}
