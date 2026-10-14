package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Service struct{ store Repository }

func New(r Repository) *Service { return &Service{r} }
func text(v string, max int, multiline bool) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > max || v == "" {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}
func (s *Service) Publish(ctx context.Context, uid string, in dto.Publish) (domain.Publication, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	if in.Audience == "" {
		in.Audience = "all_students"
	}
	if !validation.UUID(in.RequestID) || !text(in.Title, 160, false) || !text(in.Content, 5000, true) || in.Audience != "all_students" {
		return domain.Publication{}, domain.ErrInvalid
	}
	in.RequestID = strings.ToLower(in.RequestID)
	return s.store.Publish(ctx, uid, in)
}
func (s *Service) List(ctx context.Context, uid string, f dto.Filter) (domain.Page, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 {
		return domain.Page{}, domain.ErrInvalid
	}
	return s.store.List(ctx, uid, f)
}
func (s *Service) Get(ctx context.Context, uid, id string) (domain.Announcement, error) {
	if !validation.UUID(id) {
		return domain.Announcement{}, domain.ErrInvalid
	}
	return s.store.Get(ctx, uid, strings.ToLower(id))
}
func (s *Service) Summary(ctx context.Context, uid, id string) (domain.DeliverySummary, error) {
	if !validation.UUID(id) {
		return domain.DeliverySummary{}, domain.ErrInvalid
	}
	return s.store.Summary(ctx, uid, strings.ToLower(id))
}
