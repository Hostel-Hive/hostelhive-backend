package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Service struct{ store Repository }

func New(r Repository) *Service { return &Service{r} }
func textOK(v string) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > 1000 {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func (s *Service) Submit(ctx context.Context, actor string, in dto.Submit) (domain.Request, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !validation.UUID(in.RequestID) || in.StartsAt.IsZero() || in.EndsAt.IsZero() || in.StartsAt.Year() < 1 || in.EndsAt.Year() > 9999 || !in.EndsAt.After(in.StartsAt) || in.Reason == "" || !textOK(in.Reason) {
		return domain.Request{}, domain.ErrInvalid
	}
	// PostgreSQL stores microseconds; normalize payloads for exact retries.
	in.StartsAt = in.StartsAt.UTC().Truncate(time.Microsecond)
	in.EndsAt = in.EndsAt.UTC().Truncate(time.Microsecond)
	if !in.EndsAt.After(in.StartsAt) {
		return domain.Request{}, domain.ErrInvalid
	}
	in.RequestID = strings.ToLower(in.RequestID)
	return s.store.Submit(ctx, actor, in)
}
func (s *Service) Decide(ctx context.Context, actor, id string, in dto.Decision) (domain.Request, error) {
	in.Note = strings.TrimSpace(in.Note)
	if !validation.UUID(id) || (in.Status != "approved" && in.Status != "rejected") || !textOK(in.Note) || (in.Status == "rejected" && in.Note == "") {
		return domain.Request{}, domain.ErrInvalid
	}
	return s.store.Decide(ctx, actor, strings.ToLower(id), in)
}
func (s *Service) Get(ctx context.Context, actor, id string) (domain.Request, error) {
	if !validation.UUID(id) {
		return domain.Request{}, domain.ErrInvalid
	}
	return s.store.Get(ctx, actor, strings.ToLower(id))
}
func (s *Service) List(ctx context.Context, actor string, f domain.Filter) (domain.Page, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || (f.StudentID != "" && !validation.UUID(f.StudentID)) || (f.Status != "" && f.Status != "pending" && f.Status != "approved" && f.Status != "rejected") {
		return domain.Page{}, domain.ErrInvalid
	}
	f.StudentID = strings.ToLower(f.StudentID)
	return s.store.List(ctx, actor, f)
}
