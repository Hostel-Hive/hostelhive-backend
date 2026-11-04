package service

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type Repository interface {
	List(context.Context, string, d.Filter) (d.Page, error)
	Get(context.Context, string, string) (d.Faculty, error)
	Create(context.Context, string, dto.Details) (d.Faculty, error)
	Rename(context.Context, string, string, dto.Details) (d.Faculty, error)
	SetActive(context.Context, string, string, bool) (d.Faculty, error)
}
type Service struct{ store Repository }

func New(r Repository) *Service { return &Service{r} }
func normalize(v dto.Details) (dto.Details, error) {
	if !utf8.ValidString(v.Name) {
		return v, d.ErrInvalid
	}
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || utf8.RuneCountInString(v.Name) > 120 {
		return v, d.ErrInvalid
	}
	for _, r := range v.Name {
		if unicode.IsControl(r) {
			return v, d.ErrInvalid
		}
	}
	return v, nil
}
func (s *Service) List(ctx context.Context, actor string, f d.Filter) (d.Page, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 100000 {
		return d.Page{}, d.ErrInvalid
	}
	return s.store.List(ctx, actor, f)
}
func (s *Service) Get(ctx context.Context, actor, id string) (d.Faculty, error) {
	if !validation.UUID(id) {
		return d.Faculty{}, d.ErrInvalid
	}
	return s.store.Get(ctx, actor, strings.ToLower(id))
}
func (s *Service) Create(ctx context.Context, actor string, v dto.Details) (d.Faculty, error) {
	v, e := normalize(v)
	if e != nil {
		return d.Faculty{}, e
	}
	return s.store.Create(ctx, actor, v)
}
func (s *Service) Rename(ctx context.Context, actor, id string, v dto.Details) (d.Faculty, error) {
	v, e := normalize(v)
	if e != nil || !validation.UUID(id) {
		return d.Faculty{}, d.ErrInvalid
	}
	return s.store.Rename(ctx, actor, strings.ToLower(id), v)
}
func (s *Service) SetActive(ctx context.Context, actor, id string, active bool) (d.Faculty, error) {
	if !validation.UUID(id) {
		return d.Faculty{}, d.ErrInvalid
	}
	return s.store.SetActive(ctx, actor, strings.ToLower(id), active)
}
