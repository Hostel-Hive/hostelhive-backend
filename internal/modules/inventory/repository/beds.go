package repository

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/jackc/pgx/v5"
)

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
