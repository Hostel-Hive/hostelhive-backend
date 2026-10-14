package repository

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/dto"
	"time"
)

func (s *Store) Submit(ctx context.Context, uid string, in dto.Submit) (domain.Request, error) {
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
	existing, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM hostelhive.late_access_requests WHERE student_id=$1 AND request_id=$2`, a.student, in.RequestID))
	if err == nil {
		if existing.ExpectedAt.Equal(in.ExpectedAt) && existing.Direction == in.Direction && existing.Reason == in.Reason {
			return commit(ctx, tx, existing, nil)
		}
		return domain.Request{}, domain.ErrConflict
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Request{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return domain.Request{}, dbError(err)
	}
	if !in.ExpectedAt.After(now) || !s.policy.RequiresPermission(in.Direction, in.ExpectedAt) {
		return domain.Request{}, domain.ErrInvalid
	}
	var overlap bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.late_access_requests WHERE student_id=$1 AND status IN ('pending','approved') AND direction=$2 AND expected_at=$3)`, a.student, in.Direction, in.ExpectedAt).Scan(&overlap)
	if err != nil {
		return domain.Request{}, dbError(err)
	}
	if overlap {
		return domain.Request{}, domain.ErrConflict
	}
	v, err := scan(tx.QueryRow(ctx, `INSERT INTO hostelhive.late_access_requests(request_id,student_id,submitted_by,direction,expected_at,reason) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+columns, in.RequestID, a.student, a.id, in.Direction, in.ExpectedAt, in.Reason))
	return commit(ctx, tx, v, err)
}
func (s *Store) Decide(ctx context.Context, uid, id string, in dto.Decision) (domain.Request, error) {
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return domain.Request{}, err
	}
	defer rollback(tx)
	if a.role != "admin" && a.role != "warden" {
		return domain.Request{}, domain.ErrForbidden
	}
	var student string
	if err = tx.QueryRow(ctx, `SELECT student_id::text FROM hostelhive.late_access_requests WHERE late_access_id=$1`, id).Scan(&student); err != nil {
		return domain.Request{}, dbError(err)
	}
	eligible, err := lockStudent(ctx, tx, student)
	if err != nil {
		return domain.Request{}, err
	}
	v, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM hostelhive.late_access_requests WHERE late_access_id=$1 FOR UPDATE`, id))
	if err != nil {
		return domain.Request{}, err
	}
	if v.Status != "pending" {
		// Exact retries by the original actor preserve the original decision timestamp.
		if v.Status == in.Status && v.DecisionNote == in.Note && v.DecidedBy != nil && *v.DecidedBy == a.id {
			return commit(ctx, tx, v, nil)
		}
		return domain.Request{}, domain.ErrConflict
	}
	if in.Status == "approved" && !eligible {
		return domain.Request{}, domain.ErrIneligible
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return domain.Request{}, dbError(err)
	}
	if in.Status == "approved" && !v.ExpectedAt.After(now) {
		return domain.Request{}, domain.ErrConflict
	}
	v, err = scan(tx.QueryRow(ctx, `UPDATE hostelhive.late_access_requests SET status=$2,decided_by=$3,decided_at=clock_timestamp(),decision_note=$4 WHERE late_access_id=$1 RETURNING `+columns, id, in.Status, a.id, in.Note))
	// Approval grants permission only; it never changes scanned presence.
	return commit(ctx, tx, v, err)
}
func (s *Store) List(ctx context.Context, uid string, f domain.Filter) (domain.Page, error) {
	p := domain.Page{Items: []domain.Request{}, Limit: f.Limit, Offset: f.Offset}
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return p, err
	}
	defer rollback(tx)
	if a.student != "" {
		if f.StudentID != "" && f.StudentID != a.student {
			return p, domain.ErrForbidden
		}
		f.StudentID = a.student
	}
	const where = ` WHERE ($1::uuid IS NULL OR student_id=$1) AND ($2::text IS NULL OR status=$2)`
	// Count and page use one statement snapshot, including empty pages.
	rows, err := tx.Query(ctx, `WITH matching AS (SELECT `+columns+` FROM hostelhive.late_access_requests`+where+`), page AS (SELECT * FROM matching ORDER BY created_at DESC,late_access_id DESC LIMIT $3 OFFSET $4)
 SELECT (SELECT count(*) FROM matching),p.* FROM (SELECT 1) x LEFT JOIN page p ON true ORDER BY p.created_at DESC,p.late_access_id DESC`, optional(f.StudentID), optional(f.Status), f.Limit, f.Offset)
	if err != nil {
		return p, dbError(err)
	}
	for rows.Next() {
		var v domain.Request
		var id, rid, sid, submitted, reason, status, note *string
		var direction *string
		var expected, created *time.Time
		err = rows.Scan(&p.Total, &id, &rid, &sid, &submitted, &direction, &expected, &reason, &status, &created, &v.DecidedAt, &v.DecidedBy, &note)
		if err != nil {
			rows.Close()
			return p, dbError(err)
		}
		if id != nil {
			v.LateAccessID = *id
			v.RequestID = *rid
			v.StudentID = *sid
			v.SubmittedBy = *submitted
			v.Direction = *direction
			v.ExpectedAt = *expected
			v.Reason = *reason
			v.Status = *status
			v.CreatedAt = *created
			v.DecisionNote = *note
			p.Items = append(p.Items, v)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, dbError(err)
	}
	p.HasMore = int64(p.Offset)+int64(len(p.Items)) < p.Total
	if err = tx.Commit(ctx); err != nil {
		return p, dbError(err)
	}
	return p, nil
}
