package repository

import (
	"context"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"github.com/jackc/pgx/v5"
)

// OwnImages reuses the private object lifecycle without granting staff authority.
type OwnImages struct{ *Store }

func NewOwnImages(store *Store) *OwnImages { return &OwnImages{store} }

func (r *OwnImages) beginOwnImage(id string) imageBegin {
	return func(ctx context.Context, actor string) (pgx.Tx, error) {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return nil, d.ErrUnavailable
		}
		allowed := false
		defer func() {
			if !allowed {
				rollback(tx)
			}
		}()
		// Prevent account changes until attachment/removal completes. The student
		// row is locked separately by the shared lifecycle, before storage writes.
		if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
			return nil, d.ErrUnavailable
		}
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.users u
   WHERE u.firebase_uid=$1 AND u.is_active AND u.role='student'
   AND NOT EXISTS(SELECT 1 FROM hostelhive.student_invitations i WHERE i.user_id=u.user_id AND i.setup_completed_at IS NULL))`, actor).Scan(&eligible)
		if err != nil {
			return nil, d.ErrUnavailable
		}
		if !eligible {
			return nil, d.ErrForbidden
		}
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.students s
   JOIN hostelhive.users u USING(user_id) WHERE u.firebase_uid=$1 AND s.student_id=$2)`, actor, id).Scan(&eligible)
		if err != nil {
			return nil, d.ErrUnavailable
		}
		if !eligible {
			return nil, d.ErrForbidden
		}
		allowed = true
		return tx, nil
	}
}
func (r *OwnImages) ReserveImage(ctx context.Context, actor, id string, meta d.Image) error {
	return reserveImage(ctx, actor, id, meta, r.beginOwnImage(id))
}
func (r *OwnImages) BeginImage(ctx context.Context, actor, id, key string) (s.ImageWork, error) {
	return beginImage(ctx, actor, id, key, r.beginOwnImage(id))
}
func (r *OwnImages) RemoveImage(ctx context.Context, actor, id string) error {
	return removeImage(ctx, actor, id, r.beginOwnImage(id))
}
