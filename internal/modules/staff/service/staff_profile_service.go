package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"strings"
)

type Service struct{ repo Repository }

func New(r Repository) *Service { return &Service{r} }
func (s *Service) List(ctx context.Context, limit, offset int) (domain.Page, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return domain.Page{}, domain.ErrInvalid
	}
	return s.repo.List(ctx, limit, offset)
}
func (s *Service) Get(ctx context.Context, id string) (domain.Profile, error) {
	if !validation.UUID(id) {
		return domain.Profile{}, domain.ErrInvalid
	}
	return s.repo.Get(ctx, id)
}
func (s *Service) Put(ctx context.Context, actor, id string, d dto.Details) (domain.Profile, error) {
	d.FullName = strings.TrimSpace(d.FullName)
	d.Designation = strings.TrimSpace(d.Designation)
	if !validation.UUID(id) || actor == "" || !validation.Text(d.FullName, 200) || !validation.Text(d.Designation, 120) {
		return domain.Profile{}, domain.ErrInvalid
	}
	return s.repo.Put(ctx, actor, id, d)
}
