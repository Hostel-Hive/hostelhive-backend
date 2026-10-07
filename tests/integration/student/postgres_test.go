package student_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	sdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	srepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	sservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
)

func TestStudentPostgresIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL migrated to version 5")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal("test connection failed")
	}
	defer pool.Close()
	prefix := fmt.Sprintf("issue24-%d", time.Now().UnixNano())
	uids := []string{}
	userIDs := []string{}
	for n, role := range []string{"admin", "student", "student", "student", "warden"} {
		uid := fmt.Sprintf("%s-%d", prefix, n)
		var id string
		err := pool.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true) RETURNING user_id::text", uid, uid+"@example.invalid", role).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		uids = append(uids, uid)
		userIDs = append(userIDs, id)
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, sql := range []string{"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))", "DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])", "DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])"} {
			if _, err := pool.Exec(clean, sql, userIDs); err != nil {
				t.Error("fixture cleanup failed")
			}
		}
	}()
	s := sservice.New(srepo.NewStore(pool))
	input := sdto.CreateInput{UserID: userIDs[1], Details: validDetails()}
	input.IndexNo = prefix + "-original"
	original, err := s.Create(ctx, uids[0], input)
	if err != nil || len(original.Guardians) != 1 || original.UserID != userIDs[1] {
		t.Fatalf("create %v", err)
	}
	if _, err = s.Create(ctx, uids[0], input); !errors.Is(err, sdomain.ErrConflict) {
		t.Fatal("duplicate linked account accepted")
	}
	duplicate := input
	duplicate.UserID = userIDs[2]
	duplicate.IndexNo = fmt.Sprintf("ISSUE24-%d-ORIGINAL", extractTimestamp(prefix))
	if _, err = s.Create(ctx, uids[0], duplicate); !errors.Is(err, sdomain.ErrConflict) {
		t.Fatalf("case-insensitive index duplicate: %v", err)
	}
	wrong := input
	wrong.UserID = userIDs[4]
	wrong.IndexNo = prefix + "-wrong"
	if _, err = s.Create(ctx, uids[0], wrong); !errors.Is(err, sdomain.ErrAccount) {
		t.Fatal("staff account linked")
	}
	if _, err = s.Create(ctx, uids[1], wrong); !errors.Is(err, sdomain.ErrForbidden) {
		t.Fatal("stale actor accepted")
	}
	if _, err = pool.Exec(ctx, "UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", userIDs[3]); err != nil {
		t.Fatal(err)
	}
	wrong.UserID = userIDs[3]
	if _, err = s.Create(ctx, uids[0], wrong); !errors.Is(err, sdomain.ErrAccount) {
		t.Fatal("inactive student linked")
	}
	_, _ = pool.Exec(ctx, "UPDATE hostelhive.users SET is_active=true WHERE user_id=$1", userIDs[3])
	// A concurrent collision must create exactly one profile and guardian set.
	begin := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range userIDs[2:4] {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-begin
			v := sdto.CreateInput{UserID: id, Details: validDetails()}
			v.IndexNo = prefix + "-race"
			_, err := s.Create(ctx, uids[0], v)
			results <- err
		}(id)
	}
	close(begin)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, sdomain.ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("concurrent duplicates were not prevented")
	}
	page, err := s.List(ctx, sdomain.Filter{Limit: 1, Q: prefix})
	if err != nil || len(page.Students) != 1 || !page.HasMore {
		t.Fatal("pagination failed")
	}
	filtered, err := s.List(ctx, sdomain.Filter{Limit: 20, Q: prefix + "-original", Faculty: "SCIENCE", Year: 1})
	if err != nil || len(filtered.Students) != 1 || filtered.Students[0].StudentID != original.StudentID || len(filtered.Students[0].Guardians) != 0 {
		t.Fatal("search/filter or list data minimization failed")
	}
	injected, err := s.List(ctx, sdomain.Filter{Limit: 20, Q: "%' OR 1=1 --"})
	if err != nil || len(injected.Students) != 0 {
		t.Fatal("search was not literal/parameterized")
	}
	updated := input.Details
	updated.FullName = "Updated Student"
	updated.Guardians = []sdomain.GuardianInput{{Name: "Guardian One", Relationship: "Parent", ContactPhone: "0771234567"}, {Name: "Guardian Two", Relationship: "Sibling", ContactPhone: "0779876543"}}
	result, err := s.Update(ctx, uids[0], original.StudentID, updated)
	if err != nil || len(result.Guardians) != 2 || result.UserID != original.UserID || !result.UpdatedAt.After(original.UpdatedAt) {
		t.Fatal("atomic update failed")
	}
	updated.IndexNo = prefix + "-race"
	if _, err = s.Update(ctx, uids[0], original.StudentID, updated); !errors.Is(err, sdomain.ErrConflict) {
		t.Fatal("update duplicate accepted")
	}
	result, err = s.Get(ctx, original.StudentID)
	if err != nil || result.IndexNo != input.IndexNo || len(result.Guardians) != 2 {
		t.Fatal("failed update corrupted profile")
	}

	// Force a database failure after profile UPDATE and guardian replacement
	// begins, verifying rollback of both parent data and all child records.
	faultName := prefix + "-reject"
	functionName := pgx.Identifier{"hostelhive", strings.ReplaceAll(prefix, "-", "_") + "_reject_guardian"}.Sanitize()
	triggerName := pgx.Identifier{strings.ReplaceAll(prefix, "-", "_") + "_guardian_fault"}.Sanitize()
	if _, err = pool.Exec(ctx, "CREATE FUNCTION "+functionName+"() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.name=TG_ARGV[0] THEN RAISE EXCEPTION 'fixture write failure'; END IF; RETURN NEW; END $$"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "CREATE TRIGGER "+triggerName+" BEFORE INSERT ON hostelhive.guardians FOR EACH ROW EXECUTE FUNCTION "+functionName+"('"+faultName+"')"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = pool.Exec(clean, "DROP TRIGGER IF EXISTS "+triggerName+" ON hostelhive.guardians")
		_, _ = pool.Exec(clean, "DROP FUNCTION IF EXISTS "+functionName+"()")
	}()
	failed := input.Details
	failed.FullName = "Must Roll Back"
	failed.Guardians = []sdomain.GuardianInput{{Name: "Temporary Guardian", Relationship: "Parent", ContactPhone: "0771234567"}, {Name: faultName, Relationship: "Parent", ContactPhone: "0771234567"}}
	if _, err = s.Update(ctx, uids[0], original.StudentID, failed); !errors.Is(err, sdomain.ErrUnavailable) {
		t.Fatal("forced child-write failure not surfaced safely")
	}
	result, err = s.Get(ctx, original.StudentID)
	if err != nil || result.FullName != "Updated Student" || len(result.Guardians) != 2 {
		t.Fatal("partial guardian failure changed parent or children")
	}
	for _, g := range result.Guardians {
		if g.Name == "Temporary Guardian" || g.Name == faultName {
			t.Fatal("partial guardian rows committed")
		}
	}
	// A real FK reference must survive API deletion.
	historyTable := pgx.Identifier{"hostelhive", strings.ReplaceAll(prefix, "-", "_") + "_history"}.Sanitize()
	if _, err = pool.Exec(ctx, "CREATE TABLE "+historyTable+"(student_id UUID REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT)"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanup, "DROP TABLE "+historyTable); err != nil {
			t.Error("history fixture cleanup failed")
		}
	}()
	if _, err = pool.Exec(ctx, "INSERT INTO "+historyTable+" VALUES($1)", original.StudentID); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, uids[0], original.StudentID); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, uids[0], original.StudentID); err != nil {
		t.Fatal("repeated deletion failed")
	}
	var references int
	if pool.QueryRow(ctx, "SELECT count(*) FROM "+historyTable).Scan(&references) != nil || references != 1 {
		t.Fatal("history lost")
	}

	if _, err = s.Get(ctx, original.StudentID); !errors.Is(err, sdomain.ErrNotFound) {
		t.Fatal("deleted profile readable")
	}
	filtered, err = s.List(ctx, sdomain.Filter{Limit: 20, Q: input.IndexNo})
	if err != nil || len(filtered.Students) != 0 {
		t.Fatal("deleted profile listed")
	}
	if _, err = s.Update(ctx, uids[0], original.StudentID, input.Details); !errors.Is(err, sdomain.ErrNotFound) {
		t.Fatal("deleted profile updated")
	}
	if _, err = s.Create(ctx, uids[0], input); !errors.Is(err, sdomain.ErrConflict) {
		t.Fatal("archived identifiers reused")
	}
	var retained int
	var owner string
	if pool.QueryRow(ctx, "SELECT count(*),min(s.deleted_by::text) FROM hostelhive.guardians g JOIN hostelhive.students s USING(student_id) WHERE s.student_id=$1 AND s.deleted_at IS NOT NULL", original.StudentID).Scan(&retained, &owner) != nil || retained != 2 || owner != userIDs[0] {
		t.Fatal("guardians or deletion attribution lost")
	}
	var active bool
	if pool.QueryRow(ctx, "SELECT is_active FROM hostelhive.users WHERE user_id=$1", original.UserID).Scan(&active) != nil || !active {
		t.Fatal("profile deletion unexpectedly changed account access")
	}
	if err = s.Delete(ctx, uids[0], "00000000-0000-0000-0000-000000000000"); !errors.Is(err, sdomain.ErrNotFound) {
		t.Fatal("unknown profile deletion accepted")
	}
}

func extractTimestamp(prefix string) int64 {
	var timestamp int64
	_, _ = fmt.Sscanf(prefix, "issue24-%d", &timestamp)
	return timestamp
}

func validDetails() sdto.Details {
	return sdto.Details{IndexNo: "SC/2026/001", FullName: "Test Student", Faculty: "Science", Year: 1, ContactPhone: "+94 771234567", Guardians: []sdomain.GuardianInput{{Name: "Test Guardian", Relationship: "Parent", ContactPhone: "0771234567"}}}
}
