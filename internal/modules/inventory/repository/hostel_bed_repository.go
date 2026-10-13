package repository

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/dto"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateBed(ctx context.Context, actor string, in dto.CreateBed) (domain.Bed, error) {
	return write(ctx, s.pool, actor, func(tx pgx.Tx) (domain.Bed, error) {
		var b domain.Bed
		if err := tx.QueryRow(ctx, `INSERT INTO hostelhive.beds(room_id,bed_no) VALUES($1,$2) RETURNING bed_id::text,room_id::text,bed_no`, in.RoomID, in.BedNo).Scan(&b.BedID, &b.RoomID, &b.BedNo); err != nil {
			return b, err
		}
		err := tx.QueryRow(ctx, `SELECT block_id::text FROM hostelhive.rooms WHERE room_id=$1`, b.RoomID).Scan(&b.BlockID)
		b.Available = true
		return b, err
	})
}

func (s *Store) UpdateBed(ctx context.Context, actor, id string, in dto.BedDetails) (domain.Bed, error) {
	return write(ctx, s.pool, actor, func(tx pgx.Tx) (domain.Bed, error) {
		var b domain.Bed
		if err := tx.QueryRow(ctx, `UPDATE hostelhive.beds SET bed_no=$2 WHERE bed_id=$1 RETURNING bed_id::text,room_id::text,bed_no`, id, in.BedNo).Scan(&b.BedID, &b.RoomID, &b.BedNo); err != nil {
			return b, err
		}
		err := tx.QueryRow(ctx, `SELECT r.block_id::text,NOT EXISTS(SELECT 1 FROM hostelhive.bed_allocations WHERE bed_id=$1 AND ended_at IS NULL) FROM hostelhive.rooms r WHERE r.room_id=$2`, id, b.RoomID).Scan(&b.BlockID, &b.Available)
		return b, err
	})
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
