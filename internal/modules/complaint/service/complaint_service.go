package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Repository interface {
	Submit(context.Context, string, dto.Submit) (domain.Complaint, error)
	Get(context.Context, string, string) (domain.Complaint, error)
	List(context.Context, string, domain.Filter) (domain.Page, error)
	Change(context.Context, string, string, string, dto.Change) (domain.Update, error)
	Updates(context.Context, string, string, int, int) (domain.UpdatePage, error)
}
type Service struct{ store Repository }

func New(r Repository) *Service { return &Service{r} }
func validText(v string) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > 2000 {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func (s *Service) Submit(ctx context.Context, actor string, in dto.Submit) (domain.Complaint, error) {
	in.Description = strings.TrimSpace(in.Description)
	if !validation.UUID(in.RequestID) || (in.Category != "maintenance" && in.Category != "disciplinary") || in.Description == "" || !validText(in.Description) {
		return domain.Complaint{}, domain.ErrInvalid
	}
	in.RequestID = strings.ToLower(in.RequestID)
	return s.store.Submit(ctx, actor, in)
}
func (s *Service) Get(ctx context.Context, actor, id string) (domain.Complaint, error) {
	if !validation.UUID(id) {
		return domain.Complaint{}, domain.ErrInvalid
	}
	return s.store.Get(ctx, actor, strings.ToLower(id))
}
func (s *Service) List(ctx context.Context, actor string, f domain.Filter) (domain.Page, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || (f.StudentID != "" && !validation.UUID(f.StudentID)) || (f.Category != "" && f.Category != "maintenance" && f.Category != "disciplinary") || (f.Status != "" && f.Status != "pending" && f.Status != "in_progress" && f.Status != "resolved") {
		return domain.Page{}, domain.ErrInvalid
	}
	for _, v := range []*time.Time{f.From, f.To} {
		if v != nil && (v.IsZero() || v.Year() < 1 || v.Year() > 9999) {
			return domain.Page{}, domain.ErrInvalid
		}
	}
	if f.From != nil && f.To != nil && !f.To.After(*f.From) {
		return domain.Page{}, domain.ErrInvalid
	}
	f.StudentID = strings.ToLower(f.StudentID)
	return s.store.List(ctx, actor, f)
}
func (s *Service) Change(ctx context.Context, actor, id, action string, in dto.Change) (domain.Update, error) {
	in.Text = strings.TrimSpace(in.Text)
	if !validation.UUID(id) || !validation.UUID(in.RequestID) || in.ExpectedVersion < 1 || in.ExpectedVersion >= 9007199254740991 || !validText(in.Text) {
		return domain.Update{}, domain.ErrInvalid
	}
	if (action == "note" && (in.Text == "" || in.Status != "")) || (action == "status" && ((in.Status != "in_progress" && in.Status != "resolved") || (in.Status == "resolved" && in.Text == ""))) || (action != "note" && action != "status") {
		return domain.Update{}, domain.ErrInvalid
	}
	in.RequestID = strings.ToLower(in.RequestID)
	return s.store.Change(ctx, actor, strings.ToLower(id), action, in)
}
func (s *Service) Updates(ctx context.Context, actor, id string, limit, offset int) (domain.UpdatePage, error) {
	if !validation.UUID(id) || limit < 1 || limit > 100 || offset < 0 {
		return domain.UpdatePage{}, domain.ErrInvalid
	}
	return s.store.Updates(ctx, actor, strings.ToLower(id), limit, offset)
}
