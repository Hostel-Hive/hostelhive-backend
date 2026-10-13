package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type Service struct{ repo Repository }

func New(r Repository) *Service { return &Service{r} }
func valid(f domain.Filter) bool {
	return f.Limit >= 1 && f.Limit <= 100 && f.Offset >= 0 && f.Offset <= 100000 &&
		(f.BlockID == "" || validation.UUID(f.BlockID)) && (f.RoomID == "" || validation.UUID(f.RoomID))
}
func (s *Service) Blocks(ctx context.Context, f domain.Filter) (domain.Page[domain.Block], error) {
	if !valid(f) || f.BlockID != "" || f.RoomID != "" || f.Available != nil {
		return domain.Page[domain.Block]{}, domain.ErrInvalid
	}
	return s.repo.Blocks(ctx, f)
}
func (s *Service) Rooms(ctx context.Context, f domain.Filter) (domain.Page[domain.Room], error) {
	if !valid(f) || f.RoomID != "" || f.Available != nil {
		return domain.Page[domain.Room]{}, domain.ErrInvalid
	}
	return s.repo.Rooms(ctx, f)
}
func (s *Service) Beds(ctx context.Context, f domain.Filter) (domain.Page[domain.Bed], error) {
	if !valid(f) {
		return domain.Page[domain.Bed]{}, domain.ErrInvalid
	}
	return s.repo.Beds(ctx, f)
}
