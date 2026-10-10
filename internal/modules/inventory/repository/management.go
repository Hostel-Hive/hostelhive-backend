package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func writeError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return domain.ErrConflict
		case "23503":
			return domain.ErrNotFound
		case "23514", "22001", "22P02":
			return domain.ErrInvalid
		}
	}
	return domain.ErrUnavailable
}

// Account-role/status writes cannot race the fresh Admin check and commit.
func write[T any](ctx context.Context, pool *pgxpool.Pool, actor string, change func(pgx.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := pool.Begin(ctx)
	if err != nil {
		return zero, domain.ErrUnavailable
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(clean)
	}()
	if _, err = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE MODE`); err != nil {
		return zero, writeError(err)
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND role='admin' AND is_active)`, actor).Scan(&allowed); err != nil {
		return zero, writeError(err)
	}
	if !allowed {
		return zero, domain.ErrForbidden
	}
	result, err := change(tx)
	if err != nil {
		return zero, writeError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return zero, writeError(err)
	}
	return result, nil
}
