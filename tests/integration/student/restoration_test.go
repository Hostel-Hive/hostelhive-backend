package student_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStudentRestorationPostgres(t *testing.T) {
	database := os.Getenv("TEST_DATABASE_URL")
	if database == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "restore-" + uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, q := range []string{
			"ALTER TABLE hostelhive.student_profile_restorations DROP CONSTRAINT IF EXISTS restoration_test_failure",
			"DELETE FROM hostelhive.student_profile_restorations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no=$1)",
			"DELETE FROM hostelhive.student_qr WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no=$1)",
			"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no=$1)",
			"DELETE FROM hostelhive.students WHERE index_no=$1",
			"DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1 || '%'",
		} {
			// The ALTER statement has no parameter.
			var e error
			if strings.HasPrefix(q, "ALTER") {
				_, e = pool.Exec(clean, q)
			} else {
				_, e = pool.Exec(clean, q, prefix)
			}
			if e != nil {
				t.Error("cleanup", e)
			}
		}
	}()
	ids := map[string]string{}
	for _, role := range []string{"admin", "warden", "student"} {
		var id string
		if e := pool.QueryRow(ctx, `INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text`, prefix+role, prefix+role+"@example.invalid", role).Scan(&id); e != nil {
			t.Fatal(e)
		}
		ids[role] = id
	}
	store := repo.NewStore(pool)
	v, err := store.Create(ctx, prefix+"admin", dto.CreateInput{UserID: ids["student"], Details: dto.Details{IndexNo: prefix, FullName: "Restore Test Student", Faculty: "Science", Year: 2, ContactPhone: "0770000000", Guardians: []d.GuardianInput{{Name: "Restore Test Guardian", Relationship: "Mother", ContactPhone: "0770000001"}}}})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.ReplaceAll(uuid.NewString(), "-", "")
	token += token
	exec("INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,$2)", v.StudentID, token)
	if err = store.Delete(ctx, prefix+"warden", v.StudentID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Get(ctx, v.StudentID); !errors.Is(err, d.ErrNotFound) {
		t.Fatal("archive remained readable", err)
	}
	archived, err := store.ListArchived(ctx, prefix+"admin", d.Filter{Limit: 20, Q: prefix})
	if err != nil || len(archived.Students) != 1 || archived.Students[0].StudentID != v.StudentID {
		t.Fatal("archived directory", archived, err)
	}
	if _, err = store.ListArchived(ctx, prefix+"warden", d.Filter{Limit: 20}); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("Warden read archived directory", err)
	}
	if _, err = store.Restore(ctx, prefix+"warden", v.StudentID); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("repository allowed Warden restoration", err)
	}
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids["student"])
	if _, err = store.Restore(ctx, prefix+"admin", v.StudentID); !errors.Is(err, d.ErrAccount) {
		t.Fatal("inactive student restored", err)
	}
	exec("UPDATE hostelhive.users SET is_active=true,role='warden' WHERE user_id=$1", ids["student"])
	if _, err = store.Restore(ctx, prefix+"admin", v.StudentID); !errors.Is(err, d.ErrAccount) {
		t.Fatal("wrong role restored", err)
	}
	exec("UPDATE hostelhive.users SET role='student' WHERE user_id=$1", ids["student"])
	exec("ALTER TABLE hostelhive.student_profile_restorations ADD CONSTRAINT restoration_test_failure CHECK(false) NOT VALID")
	if _, err = store.Restore(ctx, prefix+"admin", v.StudentID); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("failed audit did not fail restoration", err)
	}
	if _, err = store.Get(ctx, v.StudentID); !errors.Is(err, d.ErrNotFound) {
		t.Fatal("failed restoration changed profile", err)
	}
	exec("ALTER TABLE hostelhive.student_profile_restorations DROP CONSTRAINT restoration_test_failure")
	var wg sync.WaitGroup
	results := make(chan d.RestorationResult, 4)
	failures := make(chan error, 4)
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := store.Restore(ctx, prefix+"admin", v.StudentID)
			results <- r
			failures <- e
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	restored := 0
	for r := range results {
		if r.Restored {
			restored++
		}
		if r.Student.StudentID != v.StudentID || r.Student.UserID != v.UserID || r.Student.IndexNo != v.IndexNo || len(r.Student.Guardians) != 1 || r.Student.Guardians[0].GuardianID != v.Guardians[0].GuardianID || r.Student.ProfileImageURL != "" {
			t.Fatal("identity/guardians changed", r)
		}
	}
	if restored != 1 {
		t.Fatal("concurrent restoration duplicated mutation", restored)
	}
	var count int
	var archiveActor, restoreActor string
	if e := pool.QueryRow(ctx, "SELECT count(*),min(archived_by::text),min(restored_by::text) FROM hostelhive.student_profile_restorations WHERE student_id=$1", v.StudentID).Scan(&count, &archiveActor, &restoreActor); e != nil || count != 1 || archiveActor != ids["warden"] || restoreActor != ids["admin"] {
		t.Fatal("audit", count, e)
	}
	current, err := store.Get(ctx, v.StudentID)
	if err != nil {
		t.Fatal(err)
	}
	archived, err = store.ListArchived(ctx, prefix+"admin", d.Filter{Limit: 20, Q: prefix})
	if err != nil || len(archived.Students) != 0 {
		t.Fatal("restored student remained archived", archived, err)
	}
	replay, err := store.Restore(ctx, prefix+"admin", v.StudentID)
	if err != nil || replay.Restored || !replay.Student.UpdatedAt.Equal(current.UpdatedAt) {
		t.Fatal("replay changed timestamp", replay, err)
	}
	var preserved string
	if e := pool.QueryRow(ctx, "SELECT token FROM hostelhive.student_qr WHERE student_id=$1", v.StudentID).Scan(&preserved); e != nil || preserved != token {
		t.Fatal("QR identity changed", e)
	}
	if _, err = store.Restore(ctx, prefix+"admin", uuid.NewString()); !errors.Is(err, d.ErrNotFound) {
		t.Fatal("missing profile", err)
	}
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids["admin"])
	if _, err = store.Restore(ctx, prefix+"admin", v.StudentID); !errors.Is(err, d.ErrForbidden) {
		t.Fatal("inactive Admin accepted", err)
	}
}
