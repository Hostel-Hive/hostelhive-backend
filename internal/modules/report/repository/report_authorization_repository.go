package repository

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Hold the same authority lock as account changes through each report read.
// Historical source rows are not filtered by current Student eligibility.
func beginReport(ctx context.Context, pool *pgxpool.Pool, actor string) (pgx.Tx, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
		_ = tx.Rollback(ctx)
		return nil, domain.ErrUnavailable
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden'))`, actor).Scan(&allowed); err != nil {
		_ = tx.Rollback(ctx)
		return nil, domain.ErrUnavailable
	}
	if !allowed {
		_ = tx.Rollback(ctx)
		return nil, domain.ErrForbidden
	}
	return tx, nil
}
