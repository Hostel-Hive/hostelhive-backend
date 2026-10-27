package student_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	allocation "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/domain"
	allocationdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
	allocationrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/repository"
	identityrepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	repo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type checkoutRevoker struct {
	calls int
	err   error
}

func (f *checkoutRevoker) DisableAndRevoke(context.Context, string) error { f.calls++; return f.err }

func TestStudentCheckoutPostgres(t *testing.T) {
	database := os.Getenv("TEST_DATABASE_URL")
	if database == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	prefix := "checkout-" + uuid.NewString()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	scalar := func(q string, args ...any) string {
		t.Helper()
		var out string
		if e := pool.QueryRow(ctx, q, args...).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		for _, q := range []string{
			"DELETE FROM hostelhive.student_checkouts WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.student_profile_restorations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.bed_allocations WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"UPDATE hostelhive.students SET profile_image_key=NULL WHERE index_no LIKE $1",
			"DELETE FROM hostelhive.student_image_objects WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.student_qr WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.guardians WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no LIKE $1)",
			"DELETE FROM hostelhive.students WHERE index_no LIKE $1",
			"DELETE FROM hostelhive.user_revocations WHERE user_id IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid LIKE $1)",
			"DELETE FROM hostelhive.users WHERE firebase_uid LIKE $1",
			"DELETE FROM hostelhive.beds WHERE room_id IN(SELECT room_id FROM hostelhive.rooms WHERE block_id IN(SELECT block_id FROM hostelhive.blocks WHERE name LIKE $1))",
			"DELETE FROM hostelhive.rooms WHERE block_id IN(SELECT block_id FROM hostelhive.blocks WHERE name LIKE $1)",
			"DELETE FROM hostelhive.blocks WHERE name LIKE $1",
		} {
			if _, e := pool.Exec(clean, q, prefix+"%"); e != nil {
				t.Error("cleanup", e)
			}
		}
	}()
	admin, otherAdmin, warden := prefix+"admin", prefix+"other-admin", prefix+"warden"
	for _, a := range []struct{ uid, role string }{{admin, "admin"}, {otherAdmin, "admin"}, {warden, "warden"}} {
		exec(`INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,$3,true)`, a.uid, a.uid+"@example.invalid", a.role)
	}
	adminID := scalar("SELECT user_id::text FROM hostelhive.users WHERE firebase_uid=$1", admin)
	store := repo.NewStore(pool)
	service := svc.NewCheckouts(store)
	allocations := allocationrepo.New(pool)
	block := scalar("INSERT INTO hostelhive.blocks(name) VALUES($1) RETURNING block_id::text", prefix)
	room := scalar("INSERT INTO hostelhive.rooms(block_id,room_no) VALUES($1,'101') RETURNING room_id::text", block)
	beds := []string{}
	for _, n := range []string{"01", "02", "03", "04"} {
		beds = append(beds, scalar("INSERT INTO hostelhive.beds(room_id,bed_no) VALUES($1,$2) RETURNING bed_id::text", room, n))
	}
	makeStudent := func(label string) d.Profile {
		t.Helper()
		uid := prefix + label
		userID := scalar("INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,'student',true) RETURNING user_id::text", uid, uid+"@example.invalid")
		p, e := store.Create(ctx, admin, dto.CreateInput{UserID: userID, Details: dto.Details{IndexNo: uid, FullName: "Checkout Student", Faculty: "Science", Year: 2, ContactPhone: "0770000000", Guardians: []d.GuardianInput{{Name: "Test Guardian", Relationship: "Mother", ContactPhone: "0770000001"}}}})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	assign := func(p d.Profile, bed string) allocation.Allocation {
		t.Helper()
		a, e := allocations.Assign(ctx, admin, allocationdto.Assign{StudentID: p.StudentID, BedID: bed})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	input := func(deactivate bool) dto.CheckoutInput {
		return dto.CheckoutInput{RequestID: uuid.NewString(), Reason: "graduated", Notes: "Completed clearance", DeactivateAccount: &deactivate, Confirmed: true}
	}
	run := func(p d.Profile, v dto.CheckoutInput) d.CheckoutResult {
		t.Helper()
		result, e := service.Checkout(ctx, admin, p.StudentID, v)
		if e != nil {
			t.Fatal(e)
		}
		return result
	}

	// Keep-access departure releases the bed and queues photo cleanup, retaining
	// guardians, QR and history. It never manufactures a scanner event.
	first := makeStudent("first")
	a := assign(first, beds[0])
	checkLifecycle := func(archived, hasBed bool) {
		t.Helper()
		page, e := identityrepo.NewStore(pool).List(ctx, 100, 0)
		if e != nil {
			t.Fatal("list account lifecycle", e)
		}
		for _, account := range page.Users {
			if account.UserID == adminID && (account.StudentProfileID != nil || account.StudentProfileArchived != nil || account.StudentHasActiveBed != nil) {
				t.Fatal("student metadata exposed for admin")
			}
			if account.UserID == first.UserID {
				if account.StudentProfileID == nil || *account.StudentProfileID != first.StudentID || account.StudentProfileArchived == nil || *account.StudentProfileArchived != archived || account.StudentHasActiveBed == nil || *account.StudentHasActiveBed != hasBed {
					t.Fatal("incorrect student lifecycle metadata", account)
				}
				return
			}
		}
		t.Fatal("student missing from account list")
	}
	checkLifecycle(false, true)
	photo := "student-images/" + first.StudentID + "/" + strings.ReplaceAll(uuid.NewString(), "-", "")
	exec("INSERT INTO hostelhive.student_image_objects(object_key,student_id,content_type,size_bytes,width,height,cleanup_at) VALUES($1,$2,'image/png',1,1,1,NULL)", photo, first.StudentID)
	exec("UPDATE hostelhive.students SET profile_image_key=$2 WHERE student_id=$1", first.StudentID, photo)
	qr := strings.ReplaceAll(uuid.NewString(), "-", "")
	qr += qr
	exec("INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,$2)", first.StudentID, qr)
	v := input(false)
	r := run(first, v)
	checkLifecycle(true, false)
	if r.Replayed || r.RevocationPending || !r.Checkout.AccountActiveAfter || r.Checkout.ReleasedBedID == nil || *r.Checkout.ReleasedBedID != beds[0] || r.Checkout.ReleasedAllocationID == nil || *r.Checkout.ReleasedAllocationID != a.AllocationID || r.Checkout.PerformedBy != adminID {
		t.Fatal("checkout snapshot", r)
	}
	if _, e := store.Get(ctx, first.StudentID); !errors.Is(e, d.ErrNotFound) {
		t.Fatal("profile was not archived", e)
	}
	if scalar("SELECT end_reason FROM hostelhive.bed_allocations WHERE allocation_id=$1", a.AllocationID) != "checkout" || scalar("SELECT (cleanup_at IS NOT NULL)::text FROM hostelhive.student_image_objects WHERE object_key=$1", photo) != "true" {
		t.Fatal("allocation/photo not finalized")
	}
	if scalar("SELECT token FROM hostelhive.student_qr WHERE student_id=$1", first.StudentID) != qr || scalar("SELECT count(*)::text FROM hostelhive.guardians WHERE student_id=$1", first.StudentID) != "1" || scalar("SELECT count(*)::text FROM hostelhive.attendance_events WHERE student_id=$1", first.StudentID) != "0" {
		t.Fatal("identity/history changed")
	}
	replay := run(first, v)
	if !replay.Replayed || replay.Checkout.CheckoutID != r.Checkout.CheckoutID || !replay.Checkout.PerformedAt.Equal(r.Checkout.PerformedAt) {
		t.Fatal("replay", replay)
	}
	changed := v
	changed.Notes = "different"
	if _, e := service.Checkout(ctx, admin, first.StudentID, changed); !errors.Is(e, d.ErrCheckoutConflict) {
		t.Fatal("changed replay accepted", e)
	}
	changed = v
	deactivateChoice := true
	changed.DeactivateAccount = &deactivateChoice
	if _, e := service.Checkout(ctx, admin, first.StudentID, changed); !errors.Is(e, d.ErrCheckoutConflict) {
		t.Fatal("changed account choice accepted", e)
	}
	otherTarget := makeStudent("other-target")
	if _, e := service.Checkout(ctx, admin, otherTarget.StudentID, v); !errors.Is(e, d.ErrCheckoutConflict) {
		t.Fatal("request reused for another student", e)
	}
	exec("UPDATE hostelhive.users SET role='warden' WHERE user_id=$1", otherTarget.UserID)
	if _, e := service.Checkout(ctx, admin, otherTarget.StudentID, input(false)); !errors.Is(e, d.ErrCheckoutIneligible) {
		t.Fatal("wrong-role target accepted", e)
	}
	if _, e := service.Checkout(ctx, admin, uuid.NewString(), input(false)); !errors.Is(e, d.ErrNotFound) {
		t.Fatal("missing target", e)
	}
	if _, e := service.Checkout(ctx, otherAdmin, first.StudentID, v); !errors.Is(e, d.ErrCheckoutConflict) {
		t.Fatal("other actor replay accepted", e)
	}
	if _, e := service.Checkout(ctx, admin, first.StudentID, input(false)); !errors.Is(e, d.ErrAlreadyCheckedOut) {
		t.Fatal("duplicate departure accepted", e)
	}

	// Old UUIDs remain historical replays after a legitimate return/new stay.
	if _, e := store.Restore(ctx, admin, first.StudentID); e != nil {
		t.Fatal(e)
	}
	secondAllocation := assign(first, beds[1])
	checkLifecycle(false, true)
	_ = run(first, v)
	if _, e := store.Get(ctx, first.StudentID); e != nil {
		t.Fatal("old replay archived new stay", e)
	}
	if scalar("SELECT (ended_at IS NULL)::text FROM hostelhive.bed_allocations WHERE allocation_id=$1", secondAllocation.AllocationID) != "true" {
		t.Fatal("old replay ended new stay")
	}
	if e := store.Delete(ctx, warden, first.StudentID); e != nil {
		t.Fatal(e)
	}
	if returnedStay := run(first, input(false)); !returnedStay.Checkout.ProfileWasArchived {
		t.Fatal("could not finalize a later manually archived stay")
	}

	// Deactivation is locally atomic and external failures stay in the shared
	// durable revocation queue, which can retry with a fake Firebase revoker.
	second := makeStudent("deactivate")
	_ = assign(second, beds[0])
	revocation := run(second, input(true))
	if !revocation.RevocationPending || revocation.Checkout.AccountActiveAfter {
		t.Fatal("deactivation not queued", revocation)
	}
	accounts := identityrepo.NewStore(pool)
	revoker := &checkoutRevoker{err: errors.New("provider unavailable")}
	if _, e := accounts.RetryOne(ctx, revoker, second.UserID); e == nil {
		t.Fatal("provider failure not surfaced")
	}
	if pending, e := accounts.Pending(ctx, second.UserID); e != nil || !pending {
		t.Fatal("revocation job lost", e)
	}
	revoker.err = nil
	if _, e := accounts.RetryOne(ctx, revoker, second.UserID); e != nil {
		t.Fatal(e)
	}
	if pending, e := accounts.Pending(ctx, second.UserID); e != nil || pending || revoker.calls != 2 {
		t.Fatal("revocation retry did not complete", e)
	}
	if scalar("SELECT is_active::text FROM hostelhive.users WHERE user_id=$1", second.UserID) != "false" {
		t.Fatal("revocation reactivated account")
	}

	// No bed and previously suspended accounts are valid permanent departures.
	noBed := makeStudent("no-bed")
	exec("UPDATE hostelhive.users SET is_active=false WHERE user_id=$1", noBed.UserID)
	noBedResult := run(noBed, input(false))
	if noBedResult.Checkout.ReleasedBedID != nil || noBedResult.Checkout.AccountActiveAfter {
		t.Fatal("no-bed checkout", noBedResult)
	}
	legacy := makeStudent("legacy-archive")
	_ = assign(legacy, beds[0])
	if e := store.Delete(ctx, warden, legacy.StudentID); e != nil {
		t.Fatal(e)
	}
	if legacyResult := run(legacy, input(false)); !legacyResult.Checkout.ProfileWasArchived || legacyResult.Checkout.ReleasedBedID == nil {
		t.Fatal("legacy archive not finalized", legacyResult)
	}

	// Fail the final audit insert: all preceding side effects must roll back.
	rollback := makeStudent("rollback")
	rollbackAllocation := assign(rollback, beds[0])
	rollbackPhoto := "student-images/" + rollback.StudentID + "/" + strings.ReplaceAll(uuid.NewString(), "-", "")
	exec("INSERT INTO hostelhive.student_image_objects(object_key,student_id,content_type,size_bytes,width,height,cleanup_at) VALUES($1,$2,'image/png',1,1,1,NULL)", rollbackPhoto, rollback.StudentID)
	exec("UPDATE hostelhive.students SET profile_image_key=$2 WHERE student_id=$1", rollback.StudentID, rollbackPhoto)
	exec("ALTER TABLE hostelhive.student_checkouts ADD CONSTRAINT checkout_test_failure CHECK(false) NOT VALID")
	_, failure := service.Checkout(ctx, admin, rollback.StudentID, input(true))
	exec("ALTER TABLE hostelhive.student_checkouts DROP CONSTRAINT checkout_test_failure")
	if !errors.Is(failure, d.ErrUnavailable) {
		t.Fatal("expected audit failure", failure)
	}
	if _, e := store.Get(ctx, rollback.StudentID); e != nil || scalar("SELECT is_active::text FROM hostelhive.users WHERE user_id=$1", rollback.UserID) != "true" || scalar("SELECT (ended_at IS NULL)::text FROM hostelhive.bed_allocations WHERE allocation_id=$1", rollbackAllocation.AllocationID) != "true" || scalar("SELECT count(*)::text FROM hostelhive.user_revocations WHERE user_id=$1", rollback.UserID) != "0" {
		t.Fatal("failed audit left partial changes", e)
	}
	if scalar("SELECT (cleanup_at IS NULL)::text FROM hostelhive.student_image_objects WHERE object_key=$1", rollbackPhoto) != "true" || scalar("SELECT (profile_image_key IS NOT NULL)::text FROM hostelhive.students WHERE student_id=$1", rollback.StudentID) != "true" {
		t.Fatal("failed checkout removed photo")
	}

	// Concurrent exact retries write one departure and no duplicate jobs/audits.
	concurrent := makeStudent("concurrent")
	cv := input(true)
	var wg sync.WaitGroup
	results := make(chan d.CheckoutResult, 4)
	errs := make(chan error, 4)
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x, e := service.Checkout(ctx, admin, concurrent.StudentID, cv)
			results <- x
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	original := 0
	for x := range results {
		if !x.Replayed {
			original++
		}
	}
	if original != 1 || scalar("SELECT count(*)::text FROM hostelhive.student_checkouts WHERE student_id=$1", concurrent.StudentID) != "1" {
		t.Fatal("duplicate concurrent departure")
	}

	// Allocation and checkout serialize through the existing users lock policy.
	racing := makeStudent("allocation-race")
	var assignmentErr, checkoutErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, assignmentErr = allocations.Assign(ctx, admin, allocationdto.Assign{StudentID: racing.StudentID, BedID: beds[2]})
	}()
	go func() { defer wg.Done(); _, checkoutErr = service.Checkout(ctx, admin, racing.StudentID, input(false)) }()
	wg.Wait()
	if checkoutErr != nil || (assignmentErr != nil && !errors.Is(assignmentErr, allocation.ErrIneligible)) || scalar("SELECT count(*)::text FROM hostelhive.bed_allocations WHERE student_id=$1 AND ended_at IS NULL", racing.StudentID) != "0" {
		t.Fatal("allocation race", assignmentErr, checkoutErr)
	}

	// Schema rollback must never discard departure history.
	down, e := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000032_student_checkout.down.sql"))
	if e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, rollbackErr := tx.Exec(ctx, string(down))
	_ = tx.Rollback(ctx)
	if rollbackErr == nil || !strings.Contains(rollbackErr.Error(), "Cannot roll back student checkout") {
		t.Fatal("migration rollback discarded departure history")
	}
	if scalar("SELECT count(*)::text FROM hostelhive.student_checkouts WHERE student_id=$1", first.StudentID) != "2" {
		t.Fatal("audit history changed")
	}
	if _, e := service.Checkout(ctx, warden, rollback.StudentID, input(false)); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("Warden checked out student", e)
	}
	exec("UPDATE hostelhive.users SET is_active=false WHERE firebase_uid=$1", admin)
	if _, e := service.Checkout(ctx, admin, rollback.StudentID, input(false)); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("inactive Admin checked out student", e)
	}
}
