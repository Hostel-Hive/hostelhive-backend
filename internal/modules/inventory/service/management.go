package service

import (
	"context"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

func (s *Service) CreateBlock(ctx context.Context, actor string, in dto.BlockDetails) (domain.Block, error) {
	in.Name = strings.TrimSpace(in.Name)
	if strings.TrimSpace(actor) == "" || !validation.Text(in.Name, 120) {
		return domain.Block{}, domain.ErrInvalid
	}
	return s.repo.CreateBlock(ctx, actor, in)
}

func (s *Service) UpdateBlock(ctx context.Context, actor, id string, in dto.BlockDetails) (domain.Block, error) {
	in.Name = strings.TrimSpace(in.Name)
	if strings.TrimSpace(actor) == "" || !validation.UUID(id) || !validation.Text(in.Name, 120) {
		return domain.Block{}, domain.ErrInvalid
	}
	return s.repo.UpdateBlock(ctx, actor, strings.ToLower(id), in)
}

func (s *Service) CreateRoom(ctx context.Context, actor string, in dto.CreateRoom) (domain.Room, error) {
	in.RoomNo = strings.TrimSpace(in.RoomNo)
	if strings.TrimSpace(actor) == "" || !validation.UUID(in.BlockID) || !validation.Text(in.RoomNo, 32) {
		return domain.Room{}, domain.ErrInvalid
	}
	in.BlockID = strings.ToLower(in.BlockID)
	return s.repo.CreateRoom(ctx, actor, in)
}

func (s *Service) UpdateRoom(ctx context.Context, actor, id string, in dto.RoomDetails) (domain.Room, error) {
	in.RoomNo = strings.TrimSpace(in.RoomNo)
	if strings.TrimSpace(actor) == "" || !validation.UUID(id) || !validation.Text(in.RoomNo, 32) {
		return domain.Room{}, domain.ErrInvalid
	}
	return s.repo.UpdateRoom(ctx, actor, strings.ToLower(id), in)
}

func (s *Service) CreateBed(ctx context.Context, actor string, in dto.CreateBed) (domain.Bed, error) {
	in.BedNo = strings.TrimSpace(in.BedNo)
	if strings.TrimSpace(actor) == "" || !validation.UUID(in.RoomID) || !validation.Text(in.BedNo, 32) {
		return domain.Bed{}, domain.ErrInvalid
	}
	in.RoomID = strings.ToLower(in.RoomID)
	return s.repo.CreateBed(ctx, actor, in)
}

func (s *Service) UpdateBed(ctx context.Context, actor, id string, in dto.BedDetails) (domain.Bed, error) {
	in.BedNo = strings.TrimSpace(in.BedNo)
	if strings.TrimSpace(actor) == "" || !validation.UUID(id) || !validation.Text(in.BedNo, 32) {
		return domain.Bed{}, domain.ErrInvalid
	}
	return s.repo.UpdateBed(ctx, actor, strings.ToLower(id), in)
}
