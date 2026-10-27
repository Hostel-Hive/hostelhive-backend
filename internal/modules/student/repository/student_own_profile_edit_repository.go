package repository

import (
	"context"
	"errors"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/jackc/pgx/v5"
)

func (s *Store) PatchOwnProfile(ctx context.Context, actor string, input dto.OwnProfilePatch) (d.OwnProfile, error) {
	var result d.OwnProfile
	if input.ContactPhone == nil && input.Guardians == nil {
		return result, d.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, d.ErrUnavailable
	}
	defer rollback(tx)
	// Shared with other Student edits, but excludes account/role changes and
	// staff profile mutations until commit. Never delegate authority to a client ID.
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
		return result, d.ErrUnavailable
	}
	var userID string
	err = tx.QueryRow(ctx, `SELECT u.user_id::text FROM hostelhive.users u
 WHERE u.firebase_uid=$1 AND u.is_active AND u.role='student'
 AND NOT EXISTS(SELECT 1 FROM hostelhive.student_invitations i WHERE i.user_id=u.user_id AND i.setup_completed_at IS NULL)`, actor).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, d.ErrForbidden
	}
	if err != nil {
		return result, d.ErrUnavailable
	}
	var id string
	err = tx.QueryRow(ctx, "SELECT student_id::text FROM hostelhive.students WHERE user_id=$1 AND deleted_at IS NULL FOR UPDATE", userID).Scan(&id)
	if err != nil {
		return result, dbError(err)
	}
	// Only the explicitly allowed phone column is updated. Omitted fields and
	// guardians cannot be overwritten by a concurrent partial edit.
	if _, err = tx.Exec(ctx, "UPDATE hostelhive.students SET contact_phone=COALESCE($2::text,contact_phone) WHERE student_id=$1", id, input.ContactPhone); err != nil {
		return result, dbError(err)
	}
	if input.Guardians != nil {
		if _, err = tx.Exec(ctx, "DELETE FROM hostelhive.guardians WHERE student_id=$1", id); err != nil {
			return result, d.ErrUnavailable
		}
		if err = guardians(ctx, tx, id, *input.Guardians); err != nil {
			return result, err
		}
	}
	result, err = ownProfile(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if tx.Commit(ctx) != nil {
		return d.OwnProfile{}, d.ErrUnavailable
	}
	return result, nil
}
