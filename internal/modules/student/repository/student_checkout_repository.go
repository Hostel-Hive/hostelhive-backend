package repository

import (
	"bytes"
	"context"
	"errors"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/jackc/pgx/v5"
)

const checkoutColumns = `checkout_id::text,request_id::text,student_id::text,user_id::text,reason,notes,deactivate_account,account_active_after,profile_was_archived,archived_at,released_allocation_id::text,released_bed_id::text,performed_by::text,performed_at`

func checkoutResult(ctx context.Context, tx pgx.Tx, requestID string, replayed bool) (d.CheckoutResult, error) {
	var result d.CheckoutResult
	c := &result.Checkout
	err := tx.QueryRow(ctx, `SELECT `+checkoutColumns+` FROM hostelhive.student_checkouts WHERE request_id=$1`, requestID).Scan(
		&c.CheckoutID, &c.RequestID, &c.StudentID, &c.UserID, &c.Reason, &c.Notes, &c.DeactivateAccount, &c.AccountActiveAfter, &c.ProfileWasArchived, &c.ArchivedAt, &c.ReleasedAllocationID, &c.ReleasedBedID, &c.PerformedBy, &c.PerformedAt)
	if err != nil {
		return result, dbError(err)
	}
	result.Replayed = replayed
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.user_revocations WHERE user_id=$1 AND completed_at IS NULL)`, c.UserID).Scan(&result.RevocationPending)
	if err != nil {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	return result, nil
}

func (s *Store) Checkout(ctx context.Context, actor, id string, v dto.CheckoutInput, hash []byte) (d.CheckoutResult, error) {
	if v.DeactivateAccount == nil || len(hash) != 32 || !v.Confirmed {
		return d.CheckoutResult{}, d.ErrInvalid
	}
	// Match archive/account write policy. This excludes allocation/transfer
	// transactions (users SHARE lock), so no assignment can race the departure.
	tx, err := s.beginPolicy(ctx, actor, true)
	if err != nil {
		return d.CheckoutResult{}, err
	}
	defer rollback(tx)
	var actorID string
	if err = tx.QueryRow(ctx, `SELECT user_id::text FROM hostelhive.users WHERE firebase_uid=$1`, actor).Scan(&actorID); err != nil {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	var previousHash []byte
	var previousActor string
	err = tx.QueryRow(ctx, `SELECT payload_hash,performed_by::text FROM hostelhive.student_checkouts WHERE request_id=$1`, v.RequestID).Scan(&previousHash, &previousActor)
	if err == nil {
		if previousActor != actorID || !bytes.Equal(previousHash, hash) {
			return d.CheckoutResult{}, d.ErrCheckoutConflict
		}
		result, e := checkoutResult(ctx, tx, v.RequestID, true)
		if e != nil {
			return d.CheckoutResult{}, e
		}
		if e = tx.Commit(ctx); e != nil {
			return d.CheckoutResult{}, d.ErrUnavailable
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	var userID, role string
	var active bool
	var archivedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT s.user_id::text,u.role,u.is_active,s.deleted_at FROM hostelhive.students s JOIN hostelhive.users u USING(user_id) WHERE s.student_id=$1 FOR UPDATE OF s`, id).Scan(&userID, &role, &active, &archivedAt)
	if err != nil {
		return d.CheckoutResult{}, dbError(err)
	}
	if role != "student" {
		return d.CheckoutResult{}, d.ErrCheckoutIneligible
	}
	wasArchived := archivedAt != nil
	if wasArchived {
		// Match this archive, not a departure from a previous restored stay.
		var done bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.student_checkouts WHERE student_id=$1 AND archived_at=$2)`, id, archivedAt).Scan(&done); err != nil {
			return d.CheckoutResult{}, d.ErrUnavailable
		}
		if done {
			return d.CheckoutResult{}, d.ErrAlreadyCheckedOut
		}
	}
	var happened time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&happened); err != nil {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	if archivedAt == nil {
		archivedAt = &happened
	}
	var allocationID, bedID *string
	err = tx.QueryRow(ctx, `UPDATE hostelhive.bed_allocations SET ended_at=GREATEST($2,started_at),ended_by=$3,end_reason='checkout'
 WHERE student_id=$1 AND ended_at IS NULL RETURNING allocation_id::text,bed_id::text`, id, happened, actorID).Scan(&allocationID, &bedID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE hostelhive.student_image_objects SET cleanup_at=$2 WHERE object_key=(SELECT profile_image_key FROM hostelhive.students WHERE student_id=$1)`, id, happened); err != nil {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE hostelhive.students SET profile_image_key=NULL,deleted_at=COALESCE(deleted_at,$2),deleted_by=COALESCE(deleted_by,$3) WHERE student_id=$1`, id, happened, actorID); err != nil {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	if *v.DeactivateAccount {
		if _, err = tx.Exec(ctx, `UPDATE hostelhive.users SET is_active=false WHERE user_id=$1`, userID); err != nil {
			return d.CheckoutResult{}, d.ErrUnavailable
		}
		// The existing worker disables Firebase and revokes sessions. Local access
		// denial and this durable delivery intent commit with the departure.
		if _, err = tx.Exec(ctx, `INSERT INTO hostelhive.user_revocations(user_id) VALUES($1)
   ON CONFLICT(user_id) DO UPDATE SET requested_at=clock_timestamp(),next_attempt_at=clock_timestamp(),completed_at=NULL`, userID); err != nil {
			return d.CheckoutResult{}, d.ErrUnavailable
		}
		active = false
	}
	_, err = tx.Exec(ctx, `INSERT INTO hostelhive.student_checkouts(request_id,payload_hash,student_id,user_id,reason,notes,deactivate_account,account_active_after,profile_was_archived,archived_at,released_allocation_id,released_bed_id,performed_by,performed_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, v.RequestID, hash, id, userID, v.Reason, v.Notes, *v.DeactivateAccount, active, wasArchived, archivedAt, allocationID, bedID, actorID, happened)
	if err != nil {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	result, err := checkoutResult(ctx, tx, v.RequestID, false)
	if err != nil {
		return d.CheckoutResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return d.CheckoutResult{}, d.ErrUnavailable
	}
	return result, nil
}
