package repository

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Store { return &Store{p} }

// page keeps the count and resource rows within a repeatable-read snapshot.
func page[T any](ctx context.Context, p *pgxpool.Pool, f domain.Filter, countSQL, itemsSQL string, args []any, scan func(pgx.Rows) (T, error)) (domain.Page[T], error) {
	result := domain.Page[T]{Items: make([]T, 0), Limit: f.Limit, Offset: f.Offset}
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, countSQL, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := tx.Query(ctx, itemsSQL, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		item, e := scan(rows)
		if e != nil {
			return result, e
		}
		result.Items = append(result.Items, item)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func optional(s string) any {
	if s == "" {
		return nil
	}
	return s
}
