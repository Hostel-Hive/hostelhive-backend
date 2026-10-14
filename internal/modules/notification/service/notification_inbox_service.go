package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"strings"
)

type Service struct{ store Repository }

func New(r Repository) *Service { return &Service{r} }
func (s *Service) List(ctx context.Context, uid string, f dto.Filter) (domain.Page, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 {
		return domain.Page{}, domain.ErrInvalid
	}
	return s.store.List(ctx, uid, f)
}
func (s *Service) Unread(ctx context.Context, uid string) (domain.Unread, error) {
	return s.store.Unread(ctx, uid)
}
func (s *Service) MarkRead(ctx context.Context, uid, id string) (domain.Notification, error) {
	if !validation.UUID(id) {
		return domain.Notification{}, domain.ErrInvalid
	}
	return s.store.MarkRead(ctx, uid, strings.ToLower(id))
}
