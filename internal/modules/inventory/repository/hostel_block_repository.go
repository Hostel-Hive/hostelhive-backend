package repository

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/dto"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateBlock(ctx context.Context, actor string, in dto.BlockDetails) (domain.Block, error) {
	return write(ctx, s.pool, actor, func(tx pgx.Tx) (domain.Block, error) {
		var b domain.Block
		err := tx.QueryRow(ctx, `INSERT INTO hostelhive.blocks(name) VALUES($1) RETURNING block_id::text,name`, in.Name).Scan(&b.BlockID, &b.Name)
		return b, err
	})
}

func (s *Store) UpdateBlock(ctx context.Context, actor, id string, in dto.BlockDetails) (domain.Block, error) {
	return write(ctx, s.pool, actor, func(tx pgx.Tx) (domain.Block, error) {
		var b domain.Block
		if err := tx.QueryRow(ctx, `UPDATE hostelhive.blocks SET name=$2 WHERE block_id=$1 RETURNING block_id::text,name`, id, in.Name).Scan(&b.BlockID, &b.Name); err != nil {
			return b, err
		}
		err := tx.QueryRow(ctx, `WITH summaries AS (`+roomSummary+`) SELECT count(room_id),COALESCE(sum(capacity),0)::bigint,COALESCE(sum(occupied_beds),0)::bigint,COALESCE(sum(available_beds),0)::bigint FROM summaries WHERE block_id=$1`, id).Scan(&b.RoomCount, &b.Capacity, &b.OccupiedBeds, &b.AvailableBeds)
		return b, err
	})
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
