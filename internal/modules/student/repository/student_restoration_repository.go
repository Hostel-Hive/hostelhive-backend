package repository

import (
	"context"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

func (s *Store) ListArchived(ctx context.Context, actor string, f d.Filter) (d.Page, error) {
	tx, err := s.beginPolicy(ctx, actor, true)
	if err != nil {
		return d.Page{}, err
	}
	defer rollback(tx)
	page, err := listProfiles(ctx, tx, f, true)
	if err != nil {
		return d.Page{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return d.Page{}, d.ErrUnavailable
	}
	return page, nil
}

func (s *Store) Restore(ctx context.Context, actor, id string) (d.RestorationResult, error) {
	var result d.RestorationResult
	if !validation.UUID(id) {
		return result, d.ErrInvalid
	}
	// Use the same user-authority lock as archive/create. Recheck Admin role
	// inside the transaction, including after middleware authentication.
	tx, err := s.beginPolicy(ctx, actor, true)
	if err != nil {
		return result, err
	}
	defer rollback(tx)
	var archivedAt *time.Time
	var archivedBy *string
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT s.deleted_at,s.deleted_by::text,u.is_active AND u.role='student'
 FROM hostelhive.students s JOIN hostelhive.users u USING(user_id)
 WHERE s.student_id=$1 FOR UPDATE OF s`, id).Scan(&archivedAt, &archivedBy, &eligible)
	if err != nil {
		return result, dbError(err)
	}
	if !eligible {
		return result, d.ErrAccount
	}
	if archivedAt != nil {
		// Preserve the previous archive metadata before clearing the soft-delete
		// marker. The archive's removed photo is intentionally not restored.
		_, err = tx.Exec(ctx, `INSERT INTO hostelhive.student_profile_restorations
 (student_id,archived_at,archived_by,restored_by)
 SELECT $1,$2,$3,user_id FROM hostelhive.users WHERE firebase_uid=$4`, id, archivedAt, archivedBy, actor)
		if err != nil {
			return result, d.ErrUnavailable
		}
		_, err = tx.Exec(ctx, `UPDATE hostelhive.students SET deleted_at=NULL,deleted_by=NULL WHERE student_id=$1`, id)
		if err != nil {
			return result, dbError(err)
		}
		result.Restored = true
	}
	result.Student, err = get(ctx, tx, id)
	if err != nil {
		return d.RestorationResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return d.RestorationResult{}, d.ErrUnavailable
	}
	return result, nil
}
