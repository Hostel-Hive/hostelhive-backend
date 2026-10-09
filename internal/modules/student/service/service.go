package service

import (
	"context"

	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	sdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository} }
func (s *Service) List(ctx context.Context, f sdomain.Filter) (sdomain.Page, error) {
	return s.repository.List(ctx, f)
}
func (s *Service) Get(ctx context.Context, id string) (sdomain.Profile, error) {
	return s.repository.Get(ctx, id)
}
func (s *Service) Create(ctx context.Context, actor string, input sdto.CreateInput) (sdomain.Profile, error) {
	details, err := Normalize(input.Details)
	if err != nil || !validation.UUID(input.UserID) {
		return sdomain.Profile{}, sdomain.ErrInvalid
	}
	input.Details = details
	return s.repository.Create(ctx, actor, input)
}
func (s *Service) Update(ctx context.Context, actor, id string, input sdto.Details) (sdomain.Profile, error) {
	details, err := Normalize(input)
	if err != nil || !validation.UUID(id) {
		return sdomain.Profile{}, sdomain.ErrInvalid
	}
	return s.repository.Update(ctx, actor, id, details)
}
func (s *Service) Delete(ctx context.Context, actor, id string) error {
	if !validation.UUID(id) {
		return sdomain.ErrInvalid
	}
	return s.repository.Delete(ctx, actor, id)
}
