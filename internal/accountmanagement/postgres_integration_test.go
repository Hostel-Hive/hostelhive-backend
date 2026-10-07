package accountmanagement

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAccountManagement(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires a disposable database migrated to version 4")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal("test connection failed")
	}
	defer pool.Close()
	var otherAdmins int
	if pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.users WHERE role='admin' AND is_active").Scan(&otherAdmins) != nil || otherAdmins != 0 {
		t.Fatal("integration test requires disposable database without existing admins")
	}
	prefix := fmt.Sprintf("issue22-%d", time.Now().UnixNano())
	ids := []string{}
	uids := []string{}
	for n, role := range []string{"admin", "admin", "student"} {
		uid := fmt.Sprintf("%s-%d", prefix, n)
		a, err := scan(pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING "+columns, uid, uid+"@example.invalid", role))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.UserID)
		uids = append(uids, uid)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := pool.Exec(cleanup, "DELETE FROM hostelhive.user_revocations WHERE user_id=ANY($1::uuid[])", ids)
		if err != nil {
			t.Error("job cleanup failed")
		}
		_, err = pool.Exec(cleanup, "DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])", ids)
		if err != nil {
			t.Error("fixture cleanup failed")
		}
	}()
	s := NewStore(pool)
	page, err := s.List(ctx, 1, 0)
	if err != nil || len(page.Users) != 1 || !page.HasMore {
		t.Fatal("pagination failed")
	}
	empty, err := s.List(ctx, 10, 1000000)
	if err != nil || len(empty.Users) != 0 || empty.HasMore {
		t.Fatal("empty page failed")
	}
	// Two concurrent admin self-demotions cannot remove both administrators.
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			_, err := s.Change(ctx, uids[n], ids[n], "student", false)
			results <- err
		}(n)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("unexpected concurrent failure: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("demotion successes %d", success)
	}
	var remainingID, remainingUID string
	if pool.QueryRow(ctx, "SELECT user_id::text,firebase_uid FROM hostelhive.users WHERE role='admin' AND is_active").Scan(&remainingID, &remainingUID) != nil {
		t.Fatal("lost last admin")
	}
	if _, err = s.Change(ctx, remainingUID, remainingID, "", true); !errors.Is(err, ErrLastAdmin) {
		t.Fatal("last admin deactivated")
	}
	demotedUID := uids[0]
	if remainingUID == uids[0] {
		demotedUID = uids[1]
	}
	if _, err = s.Change(ctx, demotedUID, ids[2], "warden", false); !errors.Is(err, ErrForbidden) {
		t.Fatal("stale admin privileges accepted")
	}
	changed, err := s.Change(ctx, remainingUID, ids[2], "warden", false)
	if err != nil || changed.Account.Role != "warden" {
		t.Fatal("role update failed")
	}
	fresh, err := authentication.NewPostgresAccounts(pool).FindByFirebaseUID(ctx, uids[2])
	if err != nil || fresh.Role != "warden" {
		t.Fatal("next request would see stale role")
	}
	blocked, err := s.Change(ctx, remainingUID, ids[2], "", true)
	if err != nil || blocked.Account.IsActive || !blocked.RevocationPending {
		t.Fatal("local block failed")
	}
	fresh, err = authentication.NewPostgresAccounts(pool).FindByFirebaseUID(ctx, uids[2])
	if err != nil || fresh.IsActive {
		t.Fatal("local block not visible immediately")
	}
	revoke := &fakeRevoker{fail: true}
	if worked, err := s.RetryOne(ctx, revoke, ids[2]); !worked || err == nil {
		t.Fatal("failure not retained")
	}
	var attempts int
	var complete bool
	if pool.QueryRow(ctx, "SELECT attempts,completed_at IS NOT NULL FROM hostelhive.user_revocations WHERE user_id=$1", ids[2]).Scan(&attempts, &complete) != nil || attempts != 1 || complete {
		t.Fatal("retry state incorrect")
	}
	// Migration rollback must refuse to discard unfinished revocation work.
	down, err := os.ReadFile("../../migrations/000004_user_revocations.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, string(down))
	if err == nil {
		t.Fatal("rollback discarded pending work")
	}
	_, _ = conn.Exec(ctx, "ROLLBACK")
	conn.Release()
	revoke.fail = false
	s = NewStore(pool)
	if worked, err := s.RetryOne(ctx, revoke, ids[2]); !worked || err != nil {
		t.Fatal("restart recovery failed")
	}
	pending, err := s.Pending(ctx, ids[2])
	if err != nil || pending {
		t.Fatal("completion not persisted")
	}
	if _, err = s.Change(ctx, remainingUID, "00000000-0000-0000-0000-000000000000", "warden", false); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown account accepted")
	}
	// Repeated deactivation safely creates another revocation obligation.
	if _, err = s.Change(ctx, remainingUID, ids[2], "", true); err != nil {
		t.Fatal(err)
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.RunRetries(workerCtx, revoke, time.Second, 10*time.Millisecond) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pending, err = s.Pending(ctx, ids[2])
		if err == nil && !pending {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	<-done
	if err != nil || pending {
		t.Fatal("background worker failed")
	}
}
