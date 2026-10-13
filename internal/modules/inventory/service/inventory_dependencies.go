package service

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/dto"
)

// Repository is the persistence port used by inventory use cases.
type Repository interface {
	Management
	Blocks(context.Context, domain.Filter) (domain.Page[domain.Block], error)
	Rooms(context.Context, domain.Filter) (domain.Page[domain.Room], error)
	Beds(context.Context, domain.Filter) (domain.Page[domain.Bed], error)
}

type Management interface {
	CreateBlock(context.Context, string, dto.BlockDetails) (domain.Block, error)
	UpdateBlock(context.Context, string, string, dto.BlockDetails) (domain.Block, error)
	CreateRoom(context.Context, string, dto.CreateRoom) (domain.Room, error)
	UpdateRoom(context.Context, string, string, dto.RoomDetails) (domain.Room, error)
	CreateBed(context.Context, string, dto.CreateBed) (domain.Bed, error)
	UpdateBed(context.Context, string, string, dto.BedDetails) (domain.Bed, error)
}
