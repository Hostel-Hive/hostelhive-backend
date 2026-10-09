package staff_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStaffSelfServicePostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to version 9")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal("test database connection failed")
	}
	defer pool.Close()
	prefix := fmt.Sprintf("staff-self-%d", time.Now().UnixNano())
	ids, uids := []string{}, []string{}
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		for _, sql := range []string{`DELETE FROM hostelhive.staff_profiles WHERE user_id=ANY($1::uuid[])`, `DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])`} {
			if _, e := pool.Exec(clean, sql, ids); e != nil {
				t.Error(e)
			}
		}
	}()
	for n, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		uid := fmt.Sprintf("%s-%d", prefix, n)
		var id string
		if e := pool.QueryRow(ctx, `INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, uid, uid+"@example.invalid", role).Scan(&id); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
		uids = append(uids, uid)
	}
	s := service.New(repository.New(pool))
	lookup := identityrepo.NewPostgresAccounts(pool)
	for n := range 4 {
		v, e := s.PutSelf(ctx, uids[n], dto.SelfDetails{FullName: fmt.Sprintf("  Own Staff %d  ", n)})
		if e != nil || v.UserID != ids[n] || v.FullName != fmt.Sprintf("Own Staff %d", n) || v.Designation != "" {
			t.Fatalf("self create %+v %v", v, e)
		}
		a, e := lookup.FindByFirebaseUID(ctx, uids[n])
		if e != nil || a.FullName != v.FullName || a.Designation != "" {
			t.Fatal("self /me projection")
		}
	}
	if _, e := s.PutSelf(ctx, uids[4], dto.SelfDetails{FullName: "Student"}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("Student created staff profile")
	}
	if _, e := s.PutSelf(ctx, "unknown-uid", dto.SelfDetails{FullName: "Unknown"}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("missing local account created profile")
	}
	if _, e := s.PutSelf(ctx, uids[1], dto.SelfDetails{FullName: strings.Repeat("界", 201)}); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("oversized name accepted")
	}
	before, e := s.Get(ctx, ids[1])
	if e != nil {
		t.Fatal(e)
	}
	adminBefore, e := s.Get(ctx, ids[0])
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Put(ctx, uids[0], ids[1], dto.Details{FullName: "Admin-chosen Name", Designation: "Senior Warden"}); e != nil {
		t.Fatal(e)
	}
	v, e := s.PutSelf(ctx, uids[1], dto.SelfDetails{FullName: "  My Updated Name  "})
	if e != nil || v.StaffID != before.StaffID || v.FullName != "My Updated Name" || v.Designation != "Senior Warden" || !v.CreatedAt.Equal(before.CreatedAt) {
		t.Fatalf("self update preserved designation/identity %+v %v", v, e)
	}
	adminAfter, e := s.Get(ctx, ids[0])
	if e != nil || adminAfter.FullName != adminBefore.FullName {
		t.Fatal("self write changed another account")
	}
	// Administrative designation remains authoritative regardless of name-update order.
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, e := s.PutSelf(ctx, uids[1], dto.SelfDetails{FullName: "Self Concurrent"})
		results <- e
	}()
	go func() {
		<-start
		_, e := s.Put(ctx, uids[0], ids[1], dto.Details{FullName: "Admin Concurrent", Designation: "Final Admin Designation"})
		results <- e
	}()
	close(start)
	for range 2 {
		if e := <-results; e != nil {
			t.Fatal(e)
		}
	}
	v, e = s.Get(ctx, ids[1])
	if e != nil || v.StaffID != before.StaffID || v.Designation != "Final Admin Designation" {
		t.Fatal("self/admin concurrency overwrote designation")
	}
	if _, e = pool.Exec(ctx, `UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, ids[1]); e != nil {
		t.Fatal(e)
	}
	if _, e = s.PutSelf(ctx, uids[1], dto.SelfDetails{FullName: "Inactive"}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("inactive actor created/updated profile")
	}
	if _, e = pool.Exec(ctx, `UPDATE hostelhive.users SET is_active=true,role='student' WHERE user_id=$1`, ids[1]); e != nil {
		t.Fatal(e)
	}
	if _, e = s.PutSelf(ctx, uids[1], dto.SelfDetails{FullName: "Stale staff"}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("stale staff role accepted")
	}
	v, e = s.Get(ctx, ids[1])
	if e != nil || v.Designation != "Final Admin Designation" {
		t.Fatal("role change discarded profile")
	}
	// Rolling back self-service must refuse rather than destroy unassigned profiles.
	down, e := os.ReadFile("../../../migrations/000009_staff_self_service.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	raw := string(down)
	begin, end := strings.Index(raw, "DO $$"), strings.Index(raw, "END $$;")
	if begin < 0 || end < 0 {
		t.Fatal("rollback guard missing")
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, raw[begin:end+len("END $$;")])
	_ = tx.Rollback(ctx)
	if e == nil || !strings.Contains(e.Error(), "Assign real designations") {
		t.Fatal("unsafe rollback allowed incomplete profiles")
	}
	var count int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM hostelhive.staff_profiles WHERE user_id=ANY($1::uuid[])`, ids).Scan(&count); e != nil || count != 4 {
		t.Fatal("rollback guard lost data")
	}
}
