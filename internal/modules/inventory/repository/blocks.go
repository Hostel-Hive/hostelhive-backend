package repository

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/jackc/pgx/v5"
)

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
