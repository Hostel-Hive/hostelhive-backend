package student_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	dto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	repository "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	service "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

func importDocument(inputs ...dto.CreateInput) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write(strings.Split(service.CSVHeader, ","))
	for _, in := range inputs {
		gs, _ := json.Marshal(in.Guardians)
		_ = w.Write([]string{in.UserID, in.IndexNo, in.FullName, in.Faculty, fmt.Sprint(in.Year), in.ContactPhone, string(gs)})
	}
	w.Flush()
	return b.Bytes()
}
func TestStudentCSVImportPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := fmt.Sprintf("issue38-%d", time.Now().UnixNano())
	roles := []string{"admin", "student", "student", "student", "warden", "student", "student", "student", "student", "student", "student"}
	ids, uids := []string{}, []string{}
	for i, role := range roles {
		uid := fmt.Sprintf("%s-%d", prefix, i)
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO hostelhive.users(firebase_uid,email,role,is_active)VALUES($1,$2,$3,true) RETURNING user_id::text`, uid, uid+"@example.invalid", role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		uids = append(uids, uid)
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		for _, sql := range []string{"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE user_id=ANY($1::uuid[]))", "DELETE FROM hostelhive.students WHERE user_id=ANY($1::uuid[])", "DELETE FROM hostelhive.users WHERE user_id=ANY($1::uuid[])"} {
			if _, err := pool.Exec(clean, sql, ids); err != nil {
				t.Error("fixture cleanup failed")
			}
		}
	}()
	store := repository.NewStore(pool)
	importer := service.NewImporter(store)
	input := func(n int, index string) dto.CreateInput {
		v := dto.CreateInput{UserID: ids[n], Details: validDetails()}
		v.IndexNo = prefix + "-" + index
		return v
	}
	first := input(1, "original")
	first.Guardians = append(first.Guardians, domain.GuardianInput{Name: "Second Guardian", Relationship: "Parent", ContactPhone: "0771234568"})
	bad := input(2, "bad")
	bad.Year = 99
	collision := input(2, "ORIGINAL")
	inactive := input(3, "inactive")
	wrongRole := input(4, "warden")
	good := input(5, "good")
	_, err = pool.Exec(ctx, "UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", ids[3])
	if err != nil {
		t.Fatal(err)
	}
	missing := input(2, "missing")
	missing.UserID = "00000000-0000-0000-0000-000000000001"
	raw := importDocument(first, collision, bad, inactive, wrongRole, missing, good)
	report, err := importer.Import(ctx, uids[0], raw)
	if err != nil || report.Created != 2 || report.Rejected != 5 || report.NotAttempted != 0 || report.StoppedReason != "" {
		t.Fatalf("mixed report %+v err %v", report, err)
	}
	expected := []string{"", "duplicate_in_file", "invalid_input", "active_student_account_required", "active_student_account_required", "active_student_account_required", ""}
	for i, row := range report.Rows {
		if row.Error != expected[i] {
			t.Fatalf("row %d %+v", i, row)
		}
	}
	original, err := store.Get(ctx, report.Rows[0].StudentID)
	if err != nil || len(original.Guardians) != 2 {
		t.Fatalf("guardian atomic creation %v", err)
	}
	again, err := importer.Import(ctx, uids[0], importDocument(first, good))
	if err != nil || again.Created != 0 || again.Rejected != 2 || again.Rows[0].Error != "duplicate_student" {
		t.Fatalf("safe retry %+v %v", again, err)
	}
	reread, err := store.Get(ctx, original.StudentID)
	if err != nil || !reread.CreatedAt.Equal(original.CreatedAt) || len(reread.Guardians) != 2 {
		t.Fatal("retry altered existing profile")
	}
	// Warden student CRUD remains permitted, while even a stale Admin token cannot import.
	ordinary := service.New(store)
	v, err := ordinary.Create(ctx, uids[4], input(6, "ordinary"))
	if err != nil || v.StudentID == "" {
		t.Fatalf("warden CRUD regression %v", err)
	}
	if _, err := store.CreateImport(ctx, uids[4], input(7, "denied")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("warden import %v", err)
	}
	_, err = pool.Exec(ctx, "UPDATE hostelhive.users SET role='warden' WHERE user_id=$1", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	denied, err := importer.Import(ctx, uids[0], importDocument(input(7, "stale"), input(8, "skipped")))
	if err != nil || denied.Created != 0 || denied.StoppedReason != "forbidden" || denied.NotAttempted != 1 {
		t.Fatalf("fresh authority %+v", denied)
	}
	_, err = pool.Exec(ctx, "UPDATE hostelhive.users SET role='admin',is_active=false WHERE user_id=$1", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateImport(ctx, uids[0], input(7, "inactive-admin")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("inactive Admin import")
	}
	_, err = pool.Exec(ctx, "UPDATE hostelhive.users SET is_active=true WHERE user_id=$1", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	// A guardian constraint failure must roll back its parent student as well.
	invalidGuardian := input(7, "rollback")
	invalidGuardian.Guardians[0].ContactPhone = "bad"
	if _, err = store.CreateImport(ctx, uids[0], invalidGuardian); err == nil {
		t.Fatal("invalid guardian accepted")
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.students WHERE user_id=$1", ids[7]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("orphan parent committed")
	}
	// Concurrent imports of the same identity are protected by database uniqueness.
	start := make(chan struct{})
	results := make(chan domain.ImportReport, 2)
	var wg sync.WaitGroup
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, e := importer.Import(ctx, uids[0], importDocument(input(7, "race")))
			if e != nil {
				t.Error(e)
			}
			results <- r
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	created, rejected := 0, 0
	for r := range results {
		created += r.Created
		rejected += r.Rejected
	}
	if created != 1 || rejected != 1 {
		t.Fatal("concurrent duplicate import")
	}
	// Archived identifiers remain reserved; imports must not resurrect old profiles.
	if err = ordinary.Delete(ctx, uids[0], original.StudentID); err != nil {
		t.Fatal(err)
	}
	archived, err := importer.Import(ctx, uids[0], importDocument(input(8, "original")))
	if err != nil || archived.Rows[0].Error != "duplicate_student" {
		t.Fatalf("archive reservation %+v", archived)
	}
	malformed := append(importDocument(input(9, "no-write")), []byte(`"truncated`)...)
	if _, err = importer.Import(ctx, uids[0], malformed); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("corrupt CSV accepted")
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM hostelhive.students WHERE user_id=$1", ids[9]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("structural rejection wrote profiles")
	}
}
