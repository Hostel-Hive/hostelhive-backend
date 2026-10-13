package service

import (
	"context"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type Service struct{ store Repository }

func New(r Repository) *Service { return &Service{r} }
func (s *Service) List(ctx context.Context, actor string, f domain.Filter) (domain.Page, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || (f.StudentID != "" && !validation.UUID(f.StudentID)) || (f.BedID != "" && !validation.UUID(f.BedID)) {
		return domain.Page{}, domain.ErrInvalid
	}
	f.StudentID = strings.ToLower(f.StudentID)
	f.BedID = strings.ToLower(f.BedID)
	return s.store.List(ctx, actor, f)
}
func (s *Service) Assign(ctx context.Context, actor string, in dto.Assign) (domain.Allocation, error) {
	if !validation.UUID(in.StudentID) || !validation.UUID(in.BedID) {
		return domain.Allocation{}, domain.ErrInvalid
	}
	in.StudentID = strings.ToLower(in.StudentID)
	in.BedID = strings.ToLower(in.BedID)
	return s.store.Assign(ctx, actor, in)
}
func (s *Service) Transfer(ctx context.Context, actor, id string, in dto.Transfer) (domain.Allocation, error) {
	if !validation.UUID(id) || !validation.UUID(in.BedID) {
		return domain.Allocation{}, domain.ErrInvalid
	}
	in.BedID = strings.ToLower(in.BedID)
	return s.store.Transfer(ctx, actor, strings.ToLower(id), in)
}
func (s *Service) Revoke(ctx context.Context, actor, id string) (domain.Allocation, error) {
	if !validation.UUID(id) {
		return domain.Allocation{}, domain.ErrInvalid
	}
	return s.store.Revoke(ctx, actor, strings.ToLower(id))
}
