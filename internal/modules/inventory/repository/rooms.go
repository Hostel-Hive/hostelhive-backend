package repository

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/jackc/pgx/v5"
)

// No student identity or allocation details leave the inventory API.
const roomSummary = `SELECT r.room_id,r.block_id,r.room_no,
 count(b.bed_id) AS capacity,count(a.allocation_id) AS occupied_beds,
 count(b.bed_id)-count(a.allocation_id) AS available_beds
 FROM hostelhive.rooms r LEFT JOIN hostelhive.beds b ON b.room_id=r.room_id
 LEFT JOIN hostelhive.bed_allocations a ON a.bed_id=b.bed_id AND a.ended_at IS NULL
 GROUP BY r.room_id`

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
