package repository

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/dto"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateRoom(ctx context.Context, actor string, in dto.CreateRoom) (domain.Room, error) {
	return write(ctx, s.pool, actor, func(tx pgx.Tx) (domain.Room, error) {
		var r domain.Room
		err := tx.QueryRow(ctx, `INSERT INTO hostelhive.rooms(block_id,room_no) VALUES($1,$2) RETURNING room_id::text,block_id::text,room_no`, in.BlockID, in.RoomNo).Scan(&r.RoomID, &r.BlockID, &r.RoomNo)
		return r, err
	})
}

func (s *Store) UpdateRoom(ctx context.Context, actor, id string, in dto.RoomDetails) (domain.Room, error) {
	return write(ctx, s.pool, actor, func(tx pgx.Tx) (domain.Room, error) {
		var r domain.Room
		if err := tx.QueryRow(ctx, `UPDATE hostelhive.rooms SET room_no=$2 WHERE room_id=$1 RETURNING room_id::text,block_id::text,room_no`, id, in.RoomNo).Scan(&r.RoomID, &r.BlockID, &r.RoomNo); err != nil {
			return r, err
		}
		err := tx.QueryRow(ctx, `SELECT capacity,occupied_beds,available_beds FROM (`+roomSummary+`) summaries WHERE room_id=$1`, id).Scan(&r.Capacity, &r.OccupiedBeds, &r.AvailableBeds)
		return r, err
	})
}

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
