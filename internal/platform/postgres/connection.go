package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL string, timeout time.Duration) (*pgxpool.Pool, error) {
	if timeout <= 0 {
		return nil, errors.New("DATABASE_CHECK_TIMEOUT must be positive")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// Driver errors may contain the URL/password. Do not propagate them.
		return nil, errors.New("DATABASE_URL contains invalid PostgreSQL settings")
	}
	cfg.ConnConfig.ConnectTimeout = timeout
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("could not initialize PostgreSQL pool")
	}
	return pool, nil
}
