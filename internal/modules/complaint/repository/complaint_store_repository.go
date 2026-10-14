package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Store { return &Store{p} }

type actor struct{ id, role, student string }

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
func rollback(tx pgx.Tx) {
	ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
	defer c()
	_ = tx.Rollback(ctx)
}
func optional(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (s *Store) begin(ctx context.Context, uid string) (pgx.Tx, actor, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, actor{}, domain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE MODE`); err != nil {
		rollback(tx)
		return nil, actor{}, dbError(err)
	}
	var a actor
	err = tx.QueryRow(ctx, `SELECT user_id::text,role FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden','sub_warden','student')`, uid).Scan(&a.id, &a.role)
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
func eligible(ctx context.Context, tx pgx.Tx, a actor) error {
	var yes bool
	err := tx.QueryRow(ctx, `SELECT deleted_at IS NULL FROM hostelhive.students WHERE student_id=$1 FOR UPDATE`, a.student).Scan(&yes)
	if err != nil {
		return dbError(err)
	}
	if !yes {
		return domain.ErrIneligible
	}
	return nil
}
func complaint(row pgx.Row) (domain.Complaint, error) {
	var v domain.Complaint
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return v, dbError(err)
	}
	if json.Unmarshal(raw, &v) != nil {
		return v, domain.ErrUnavailable
	}
	return v, nil
}
func update(row pgx.Row) (domain.Update, error) {
	var v domain.Update
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return v, dbError(err)
	}
	if json.Unmarshal(raw, &v) != nil {
		return v, domain.ErrUnavailable
	}
	return v, nil
}
func get(ctx context.Context, tx pgx.Tx, a actor, id string, lock bool) (domain.Complaint, error) {
	sql := `SELECT to_jsonb(c) FROM hostelhive.complaints c WHERE complaint_id=$1 AND ($2::uuid IS NULL OR student_id=$2) AND (NOT $3::boolean OR category='maintenance')`
	if lock {
		sql += ` FOR UPDATE OF c`
	}
	return complaint(tx.QueryRow(ctx, sql, id, optional(a.student), a.role == "sub_warden"))
}
func (s *Store) Get(ctx context.Context, uid, id string) (domain.Complaint, error) {
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return domain.Complaint{}, err
	}
	defer rollback(tx)
	v, err := get(ctx, tx, a, id, false)
	if err != nil {
		return v, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Complaint{}, dbError(err)
	}
	return v, nil
}
