package repository

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct {
	pool   *pgxpool.Pool
	policy domain.Policy
}

func New(p *pgxpool.Pool, policy domain.Policy) *Store { return &Store{pool: p, policy: policy} }

const columns = `late_access_id::text,request_id::text,student_id::text,submitted_by::text,direction,expected_at,reason,status,created_at,decided_at,decided_by::text,decision_note`

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
func scan(row pgx.Row) (domain.Request, error) {
	var v domain.Request
	err := row.Scan(&v.LateAccessID, &v.RequestID, &v.StudentID, &v.SubmittedBy, &v.Direction, &v.ExpectedAt, &v.Reason, &v.Status, &v.CreatedAt, &v.DecidedAt, &v.DecidedBy, &v.DecisionNote)
	if err != nil {
		return v, dbError(err)
	}
	return v, nil
}
func rollback(tx pgx.Tx) {
	ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
	defer c()
	_ = tx.Rollback(ctx)
}

type actor struct{ id, role, student string }

func (s *Store) begin(ctx context.Context, uid string) (pgx.Tx, actor, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, actor{}, domain.ErrUnavailable
	}
	// Account mutations cannot race the current permission check and commit.
	if _, err = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE MODE`); err != nil {
		rollback(tx)
		return nil, actor{}, dbError(err)
	}
	var a actor
	err = tx.QueryRow(ctx, `SELECT user_id::text,role FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden','student')`, uid).Scan(&a.id, &a.role)
	if err != nil {
		rollback(tx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, a, domain.ErrForbidden
		}
		return nil, a, dbError(err)
	}
	if a.role == "student" {
		err = tx.QueryRow(ctx, `SELECT student_id::text FROM hostelhive.students WHERE user_id=$1`, a.id).Scan(&a.student)
		if err != nil {
			rollback(tx)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, a, domain.ErrIneligible
			}
			return nil, a, dbError(err)
		}
	}
	return tx, a, nil
}
func lockStudent(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var active bool
	err := tx.QueryRow(ctx, `SELECT s.deleted_at IS NULL AND u.is_active AND u.role='student' FROM hostelhive.students s JOIN hostelhive.users u USING(user_id) WHERE s.student_id=$1 FOR UPDATE OF s`, id).Scan(&active)
	if err != nil {
		return false, dbError(err)
	}
	return active, nil
}
func optional(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func commit(ctx context.Context, tx pgx.Tx, v domain.Request, err error) (domain.Request, error) {
	if err != nil {
		return domain.Request{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Request{}, dbError(err)
	}
	return v, nil
}
func (s *Store) Get(ctx context.Context, uid, id string) (domain.Request, error) {
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return domain.Request{}, err
	}
	defer rollback(tx)
	// Another student's record is concealed as not found.
	v, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM hostelhive.late_access_requests WHERE late_access_id=$1 AND ($2::uuid IS NULL OR student_id=$2)`, id, optional(a.student)))
	return commit(ctx, tx, v, err)
}
