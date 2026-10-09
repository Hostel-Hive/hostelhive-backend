package repository

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Store { return &Store{p} }

// No student identity or allocation details leave the inventory API.
const roomSummary = `SELECT r.room_id,r.block_id,r.room_no,
 count(b.bed_id) AS capacity,count(a.allocation_id) AS occupied_beds,
 count(b.bed_id)-count(a.allocation_id) AS available_beds
 FROM hostelhive.rooms r LEFT JOIN hostelhive.beds b ON b.room_id=r.room_id
 LEFT JOIN hostelhive.bed_allocations a ON a.bed_id=b.bed_id AND a.ended_at IS NULL
 GROUP BY r.room_id`

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
func (s *Store) Blocks(ctx context.Context, f domain.Filter) (domain.Page[domain.Block], error) {
	return page(ctx, s.pool, f, `SELECT count(*) FROM hostelhive.blocks`,
		`WITH summaries AS (`+roomSummary+`) SELECT b.block_id::text,b.name,count(r.room_id),
 COALESCE(sum(r.capacity),0)::bigint,COALESCE(sum(r.occupied_beds),0)::bigint,COALESCE(sum(r.available_beds),0)::bigint
 FROM hostelhive.blocks b LEFT JOIN summaries r ON r.block_id=b.block_id
 GROUP BY b.block_id ORDER BY lower(b.name),b.block_id LIMIT $1 OFFSET $2`, nil,
		func(rows pgx.Rows) (domain.Block, error) {
			var x domain.Block
			err := rows.Scan(&x.BlockID, &x.Name, &x.RoomCount, &x.Capacity, &x.OccupiedBeds, &x.AvailableBeds)
			return x, err
		})
}
func (s *Store) Rooms(ctx context.Context, f domain.Filter) (domain.Page[domain.Room], error) {
	return page(ctx, s.pool, f, `SELECT count(*) FROM hostelhive.rooms WHERE ($1::uuid IS NULL OR block_id=$1)`,
		`WITH summaries AS (`+roomSummary+`) SELECT room_id::text,block_id::text,room_no,capacity,occupied_beds,available_beds
 FROM summaries WHERE ($1::uuid IS NULL OR block_id=$1) ORDER BY block_id,lower(room_no),room_id LIMIT $2 OFFSET $3`, []any{optional(f.BlockID)},
		func(rows pgx.Rows) (domain.Room, error) {
			var x domain.Room
			err := rows.Scan(&x.RoomID, &x.BlockID, &x.RoomNo, &x.Capacity, &x.OccupiedBeds, &x.AvailableBeds)
			return x, err
		})
}
func optional(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func (s *Store) Beds(ctx context.Context, f domain.Filter) (domain.Page[domain.Bed], error) {
	const base = ` FROM hostelhive.beds b JOIN hostelhive.rooms r ON r.room_id=b.room_id
 LEFT JOIN hostelhive.bed_allocations a ON a.bed_id=b.bed_id AND a.ended_at IS NULL
 WHERE ($1::uuid IS NULL OR r.block_id=$1) AND ($2::uuid IS NULL OR b.room_id=$2)
 AND ($3::boolean IS NULL OR (a.allocation_id IS NULL)=$3)`
	return page(ctx, s.pool, f, `SELECT count(*)`+base,
		`SELECT b.bed_id::text,b.room_id::text,r.block_id::text,b.bed_no,a.allocation_id IS NULL`+base+
			` ORDER BY r.block_id,b.room_id,lower(b.bed_no),b.bed_id LIMIT $4 OFFSET $5`, []any{optional(f.BlockID), optional(f.RoomID), f.Available},
		func(rows pgx.Rows) (domain.Bed, error) {
			var x domain.Bed
			err := rows.Scan(&x.BedID, &x.RoomID, &x.BlockID, &x.BedNo, &x.Available)
			return x, err
		})
}
