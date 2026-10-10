package service

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
)

// Repository is the persistence port used by inventory listing rules.
type Repository interface {
	Blocks(context.Context, domain.Filter) (domain.Page[domain.Block], error)
	Rooms(context.Context, domain.Filter) (domain.Page[domain.Room], error)
	Beds(context.Context, domain.Filter) (domain.Page[domain.Bed], error)
}
