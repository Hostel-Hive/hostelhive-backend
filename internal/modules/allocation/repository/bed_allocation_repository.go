package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Store { return &Store{p} }

const columns = `allocation_id::text,student_id::text,bed_id::text,started_at,ended_at,started_by::text,ended_by::text,end_reason`

func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && (p.Code == "23505" || p.Code == "40P01" || p.Code == "40001") {
		return domain.ErrConflict
	}
	return domain.ErrUnavailable
}
func scan(row pgx.Row) (domain.Allocation, error) {
	var a domain.Allocation
	err := row.Scan(&a.AllocationID, &a.StudentID, &a.BedID, &a.StartedAt, &a.EndedAt, &a.StartedBy, &a.EndedBy, &a.EndReason)
	if err != nil {
		return a, dbError(err)
	}
	return a, nil
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// SHARE permits concurrent allocations but blocks existing account/profile write
// policies until authorization, eligibility and the allocation commit are complete.
func (s *Store) begin(ctx context.Context, actor string) (pgx.Tx, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", domain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE MODE`); err != nil {
		rollback(tx)
		return nil, "", dbError(err)
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden')`, actor).Scan(&id)
	if err != nil {
		rollback(tx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", domain.ErrForbidden
		}
		return nil, "", dbError(err)
	}
	return tx, id, nil
}
func lockStudent(ctx context.Context, tx pgx.Tx, id string, eligible bool) error {
	var deleted *time.Time
	var role string
	var active bool
	err := tx.QueryRow(ctx, `SELECT s.deleted_at,u.role,u.is_active FROM hostelhive.students s JOIN hostelhive.users u USING(user_id) WHERE s.student_id=$1 FOR UPDATE OF s`, id).Scan(&deleted, &role, &active)
	if err != nil {
		return dbError(err)
	}
	if eligible && (deleted != nil || role != "student" || !active) {
		return domain.ErrIneligible
	}
	return nil
}
func lockBeds(ctx context.Context, tx pgx.Tx, first, second string) error {
	// Stable ordering prevents cross-bed transfers from taking reversed locks.
	rows, err := tx.Query(ctx, `SELECT bed_id::text FROM hostelhive.beds WHERE bed_id=$1 OR bed_id=$2 ORDER BY bed_id FOR UPDATE`, first, second)
	if err != nil {
		return dbError(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return dbError(err)
		}
		n++
	}
	if err = rows.Err(); err != nil {
		return dbError(err)
	}
	want := 2
	if first == second {
		want = 1
	}
	if n != want {
		return domain.ErrNotFound
	}
	return nil
}
func insert(ctx context.Context, tx pgx.Tx, actor, student, bed string) (domain.Allocation, error) {
	return scan(tx.QueryRow(ctx, `INSERT INTO hostelhive.bed_allocations(student_id,bed_id,started_by) VALUES($1,$2,$3) RETURNING `+columns, student, bed, actor))
}
func (s *Store) Assign(ctx context.Context, actor string, in dto.Assign) (domain.Allocation, error) {
	tx, uid, err := s.begin(ctx, actor)
	if err != nil {
		return domain.Allocation{}, err
	}
	defer rollback(tx)
	if err = lockStudent(ctx, tx, in.StudentID, true); err != nil {
		return domain.Allocation{}, err
	}
	if err = lockBeds(ctx, tx, in.BedID, in.BedID); err != nil {
		return domain.Allocation{}, err
	}
	a, err := insert(ctx, tx, uid, in.StudentID, in.BedID)
	if err != nil {
		return a, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Allocation{}, dbError(err)
	}
	return a, nil
}
func (s *Store) change(ctx context.Context, actor, id, bed string, transfer bool) (domain.Allocation, error) {
	tx, uid, err := s.begin(ctx, actor)
	if err != nil {
		return domain.Allocation{}, err
	}
	defer rollback(tx)
	// Discover the owner before taking locks; all mutations lock student first.
	var student string
	if err = tx.QueryRow(ctx, `SELECT student_id::text FROM hostelhive.bed_allocations WHERE allocation_id=$1`, id).Scan(&student); err != nil {
		return domain.Allocation{}, dbError(err)
	}
	if err = lockStudent(ctx, tx, student, transfer); err != nil {
		return domain.Allocation{}, err
	}
	current, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM hostelhive.bed_allocations WHERE allocation_id=$1 FOR UPDATE`, id))
	if err != nil {
		return current, err
	}
	if current.EndedAt != nil {
		if !transfer && current.EndReason != nil && *current.EndReason == "revoked" {
			if err = tx.Commit(ctx); err != nil {
				return domain.Allocation{}, dbError(err)
			}
			return current, nil
		}
		return domain.Allocation{}, domain.ErrConflict
	}
	reason := "revoked"
	if transfer {
		if current.BedID == bed {
			return domain.Allocation{}, domain.ErrConflict
		}
		if err = lockBeds(ctx, tx, current.BedID, bed); err != nil {
			return domain.Allocation{}, err
		}
		reason = "transferred"
	}
	ended, err := scan(tx.QueryRow(ctx, `UPDATE hostelhive.bed_allocations SET ended_at=GREATEST(clock_timestamp(),started_at),ended_by=$2,end_reason=$3 WHERE allocation_id=$1 RETURNING `+columns, id, uid, reason))
	if err != nil {
		return ended, err
	}
	result := ended
	if transfer {
		result, err = insert(ctx, tx, uid, student, bed)
		if err != nil {
			return domain.Allocation{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Allocation{}, dbError(err)
	}
	return result, nil
}
func (s *Store) Transfer(ctx context.Context, actor, id string, in dto.Transfer) (domain.Allocation, error) {
	return s.change(ctx, actor, id, in.BedID, true)
}
func (s *Store) Revoke(ctx context.Context, actor, id string) (domain.Allocation, error) {
	return s.change(ctx, actor, id, "", false)
}
func optional(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (s *Store) List(ctx context.Context, actor string, f domain.Filter) (domain.Page, error) {
	p := domain.Page{Items: []domain.Allocation{}, Limit: f.Limit, Offset: f.Offset}
	// A single aggregate query gives count and page the same statement snapshot.
	tx, _, err := s.begin(ctx, actor)
	if err != nil {
		return p, err
	}
	defer rollback(tx)
	// Repeatable read is unnecessary: count and rows are taken by the same query.
	const where = ` WHERE ($1::uuid IS NULL OR student_id=$1) AND ($2::uuid IS NULL OR bed_id=$2) AND ($3::boolean IS NULL OR (ended_at IS NULL)=$3)`
	rows, err := tx.Query(ctx, `WITH matching AS (SELECT `+columns+` FROM hostelhive.bed_allocations`+where+`), page AS (SELECT * FROM matching ORDER BY started_at DESC,allocation_id DESC LIMIT $4 OFFSET $5)
 SELECT (SELECT count(*) FROM matching),p.allocation_id,p.student_id,p.bed_id,p.started_at,p.ended_at,p.started_by,p.ended_by,p.end_reason FROM (SELECT 1) x LEFT JOIN page p ON true ORDER BY p.started_at DESC,p.allocation_id DESC`, optional(f.StudentID), optional(f.BedID), f.Active, f.Limit, f.Offset)
	if err != nil {
		return p, dbError(err)
	}
	for rows.Next() {
		var a domain.Allocation
		var aid, sid, bid *string
		var started *time.Time
		err = rows.Scan(&p.Total, &aid, &sid, &bid, &started, &a.EndedAt, &a.StartedBy, &a.EndedBy, &a.EndReason)
		if err != nil {
			rows.Close()
			return p, dbError(err)
		}
		if aid != nil {
			a.AllocationID = *aid
			a.StudentID = *sid
			a.BedID = *bid
			a.StartedAt = *started
			p.Items = append(p.Items, a)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return p, dbError(err)
	}
	return p, nil
}
