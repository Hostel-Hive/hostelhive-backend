package repository

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
)

// Cancel follows the same account -> Student -> request lock order as decisions
// and scan consumption. Ownership is in the locked query, never client input.
func (s *Store) Cancel(ctx context.Context, uid, id string) (domain.Request, error) {
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return domain.Request{}, err
	}
	defer rollback(tx)
	if a.role != "student" {
		return domain.Request{}, domain.ErrForbidden
	}
	eligible, err := lockStudent(ctx, tx, a.student)
	if err != nil {
		return domain.Request{}, err
	}
	if !eligible {
		return domain.Request{}, domain.ErrIneligible
	}
	v, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM hostelhive.leave_requests WHERE leave_id=$1 AND student_id=$2 FOR UPDATE`, id, a.student))
	if err != nil {
		return domain.Request{}, err
	}
	if v.Status == "cancelled" {
		return commit(ctx, tx, v, nil)
	}
	if v.Status != "pending" {
		return domain.Request{}, domain.ErrConflict
	}
	v, err = scan(tx.QueryRow(ctx, `UPDATE hostelhive.leave_requests SET status='cancelled',cancelled_at=clock_timestamp(),cancelled_by=$2 WHERE leave_id=$1 RETURNING `+columns, id, a.id))
	// No presence, attendance event, decision or notification is manufactured.
	return commit(ctx, tx, v, err)
}
