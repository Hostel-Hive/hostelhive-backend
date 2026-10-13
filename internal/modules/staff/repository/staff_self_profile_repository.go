package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/jackc/pgx/v5"
)

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
