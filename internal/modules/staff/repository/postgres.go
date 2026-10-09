package repository

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Store { return &Store{p} }

const columns = `s.staff_id::text,s.user_id::text,s.full_name,s.designation,u.email,u.role,u.is_active,s.created_at,s.updated_at`
const source = ` FROM hostelhive.staff_profiles s JOIN hostelhive.users u USING(user_id)`

func scan(row pgx.Row) (domain.Profile, error) {
	var s domain.Profile
	err := row.Scan(&s.StaffID, &s.UserID, &s.FullName, &s.Designation, &s.Email, &s.Role, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, domain.ErrNotFound
	}
	if err != nil {
		return s, domain.ErrUnavailable
	}
	return s, nil
}
func (s *Store) Get(ctx context.Context, id string) (domain.Profile, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+source+` WHERE s.user_id=$1`, id))
}
func (s *Store) List(ctx context.Context, limit, offset int) (domain.Page, error) {
	result := domain.Page{Staff: []domain.Profile{}, Limit: limit, Offset: offset}
	rows, err := s.pool.Query(ctx, `SELECT `+columns+source+` ORDER BY s.created_at,s.staff_id LIMIT $1 OFFSET $2`, limit+1, offset)
	if err != nil {
		return result, domain.ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return result, e
		}
		result.Staff = append(result.Staff, v)
	}
	if rows.Err() != nil {
		return result, domain.ErrUnavailable
	}
	if len(result.Staff) > limit {
		result.HasMore = true
		result.Staff = result.Staff[:limit]
	}
	return result, nil
}
func (s *Store) Put(ctx context.Context, actor, id string, d dto.Details) (domain.Profile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(clean)
	}()
	// Same account-write ordering as role changes, activation and student writes.
	if _, err = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND role='admin' AND is_active)`, actor).Scan(&allowed); err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	if !allowed {
		return domain.Profile{}, domain.ErrForbidden
	}
	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM hostelhive.users WHERE user_id=$1`, id).Scan(&role); errors.Is(err, pgx.ErrNoRows) {
		return domain.Profile{}, domain.ErrNotFound
	} else if err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	if role != "admin" && role != "warden" && role != "sub_warden" && role != "security_staff" {
		return domain.Profile{}, domain.ErrStaffRequired
	}
	if _, err = tx.Exec(ctx, `INSERT INTO hostelhive.staff_profiles(user_id,full_name,designation) VALUES($1,$2,$3)
 ON CONFLICT(user_id) DO UPDATE SET full_name=excluded.full_name,designation=excluded.designation`, id, d.FullName, d.Designation); err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	result, err := scan(tx.QueryRow(ctx, `SELECT `+columns+source+` WHERE s.user_id=$1`, id))
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	return result, nil
}

// PutSelf derives ownership from the verified UID and fresh database state.
// It never accepts a target ID or modifies an existing designation.
func (s *Store) PutSelf(ctx context.Context, actor string, d dto.SelfDetails) (domain.Profile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(clean)
	}()
	if _, err = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM hostelhive.users WHERE firebase_uid=$1 AND is_active
 AND role IN('admin','warden','sub_warden','security_staff')`, actor).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Profile{}, domain.ErrForbidden
	}
	if err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO hostelhive.staff_profiles(user_id,full_name) VALUES($1,$2)
 ON CONFLICT(user_id) DO UPDATE SET full_name=excluded.full_name`, id, d.FullName); err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	result, err := scan(tx.QueryRow(ctx, `SELECT `+columns+source+` WHERE s.user_id=$1`, id))
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	return result, nil
}
