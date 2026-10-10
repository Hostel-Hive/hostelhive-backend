package staff_test

import (
	"context"
	"errors"
	"fmt"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func TestStaffProfilesPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable database migrated to version 8")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal("connection failed")
	}
	defer pool.Close()
	prefix := fmt.Sprintf("staff36-%d", time.Now().UnixNano())
	ids, uids := []string{}, []string{}
	defer func() {
		clean, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		for _, sql := range []string{`DELETE FROM hostelhive.staff_profiles WHERE user_id=ANY($1::uuid[])`, `DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])`, `DELETE FROM hostelhive.user_revocations WHERE user_id=ANY($1::uuid[])`, `DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])`} {
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
	management := identityrepo.NewStore(pool)
	for n := range 4 {
		v, e := s.Put(ctx, uids[0], ids[n], dto.Details{FullName: fmt.Sprintf("Staff %d", n), Designation: "Hostel Staff"})
		if e != nil || v.UserID != ids[n] || v.StaffID == "" {
			t.Fatalf("create profile: %v", e)
		}
		a, e := lookup.FindByFirebaseUID(ctx, uids[n])
		if e != nil || a.FullName != v.FullName || a.Designation != v.Designation {
			t.Fatalf("self profile: %+v %v", a, e)
		}
	}
	if _, e := s.Put(ctx, uids[0], ids[4], dto.Details{FullName: "Student", Designation: "Staff"}); !errors.Is(e, domain.ErrStaffRequired) {
		t.Fatal("student staff profile accepted")
	}
	if _, e := s.Put(ctx, uids[1], ids[1], dto.Details{FullName: "Self", Designation: "Admin"}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("stale non-Admin actor accepted")
	}
	if _, e := s.Get(ctx, "00000000-0000-4000-8000-000000000001"); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal("missing profile accepted")
	}
	before, e := s.Get(ctx, ids[1])
	if e != nil {
		t.Fatal(e)
	}
	after, e := s.Put(ctx, uids[0], ids[1], dto.Details{FullName: "  Updated Warden  ", Designation: "  Warden  "})
	if e != nil || after.StaffID != before.StaffID || !after.CreatedAt.Equal(before.CreatedAt) || !after.UpdatedAt.After(before.UpdatedAt) || after.FullName != "Updated Warden" {
		t.Fatalf("update identity/timestamps: %+v %v", after, e)
	}
	if _, e = pool.Exec(ctx, `UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, ids[1]); e != nil {
		t.Fatal(e)
	}
	v, e := s.Get(ctx, ids[1])
	if e != nil || v.IsActive || v.FullName != "Updated Warden" {
		t.Fatal("deactivation lost profile")
	}
	if _, e = s.Put(ctx, uids[0], ids[1], dto.Details{FullName: "Updated Warden", Designation: "Senior Warden"}); e != nil {
		t.Fatal("Admin cannot edit inactive target")
	}
	if _, e = pool.Exec(ctx, `UPDATE hostelhive.users SET is_active=true WHERE user_id=$1`, ids[1]); e != nil {
		t.Fatal(e)
	}
	if _, e = management.Change(ctx, uids[0], ids[1], "student", false); e != nil {
		t.Fatal(e)
	}
	a, e := lookup.FindByFirebaseUID(ctx, uids[1])
	if e != nil || a.FullName != "" || a.Designation != "" {
		t.Fatal("former staff profile leaked into Student /me")
	}
	if _, e = s.Put(ctx, uids[0], ids[1], dto.Details{FullName: "New", Designation: "Student"}); !errors.Is(e, domain.ErrStaffRequired) {
		t.Fatal("former staff edited as Student")
	}
	var studentID string
	if e = pool.QueryRow(ctx, `INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,'Student Name','Science',1,'0771234567') RETURNING student_id::text`, ids[1], prefix).Scan(&studentID); e != nil {
		t.Fatal(e)
	}
	a, e = lookup.FindByFirebaseUID(ctx, uids[1])
	if e != nil || a.FullName != "Student Name" || a.Designation != "" {
		t.Fatal("student name not projected")
	}
	if _, e = pool.Exec(ctx, `UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by=$2 WHERE student_id=$1`, studentID, ids[0]); e != nil {
		t.Fatal(e)
	}
	a, e = lookup.FindByFirebaseUID(ctx, uids[1])
	if e != nil || a.FullName != "" {
		t.Fatal("deleted student name exposed")
	}
	if _, e = management.Change(ctx, uids[0], ids[1], "warden", false); e != nil {
		t.Fatal(e)
	}
	a, e = lookup.FindByFirebaseUID(ctx, uids[1])
	if e != nil || a.FullName != "Updated Warden" || a.Designation != "Senior Warden" {
		t.Fatal("role roundtrip lost staff data")
	}
	users, e := management.List(ctx, 100, 0)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, u := range users.Users {
		if u.UserID == ids[1] {
			found = true
			if u.FullName != a.FullName || u.Designation != a.Designation {
				t.Fatal("account list and /me names disagree")
			}
		}
	}
	if !found {
		t.Fatal("account list missing target")
	}
	page, e := s.List(ctx, 1, 0)
	if e != nil || len(page.Staff) != 1 || !page.HasMore {
		t.Fatal("staff pagination")
	}
	next, e := s.List(ctx, 1, 1)
	if e != nil || next.Staff[0].StaffID == page.Staff[0].StaffID {
		t.Fatal("duplicate page")
	}
	// Concurrent PUTs converge on one profile without changing its identity.
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, name := range []string{"Concurrent A", "Concurrent B"} {
		go func(name string) {
			<-start
			_, e := s.Put(ctx, uids[0], ids[1], dto.Details{FullName: name, Designation: "Warden"})
			results <- e
		}(name)
	}
	close(start)
	for range 2 {
		if e := <-results; e != nil {
			t.Fatal(e)
		}
	}
	v, e = s.Get(ctx, ids[1])
	if e != nil || v.StaffID != before.StaffID {
		t.Fatal("concurrent PUT replaced profile")
	}
	var count int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM hostelhive.staff_profiles WHERE user_id=$1`, ids[1]).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate staff profile")
	}
	if _, e = pool.Exec(ctx, `INSERT INTO hostelhive.staff_profiles(user_id,full_name,designation) VALUES($1,'Duplicate','Warden')`, ids[1]); e == nil {
		t.Fatal("unique user FK missing")
	}
	if _, e = pool.Exec(ctx, `DELETE FROM hostelhive.users WHERE user_id=$1`, ids[2]); e == nil {
		t.Fatal("staff history FK missing")
	}
	// Revoking actor's role after middleware verification still blocks the write.
	if _, e = pool.Exec(ctx, `UPDATE hostelhive.users SET role='warden' WHERE user_id=$1`, ids[0]); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Put(ctx, uids[0], ids[1], dto.Details{FullName: "Stale Admin", Designation: "Warden"}); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("stale Admin role accepted")
	}
}
