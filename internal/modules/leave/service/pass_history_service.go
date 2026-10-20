package service

import (
	"context"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
)

type PassHistoryRepository interface {
	OwnPasses(context.Context, string, domain.PassFilter) (domain.PassPage, error)
}

type PassHistoryReader interface {
	ListPasses(context.Context, string, domain.PassFilter) (domain.PassPage, error)
}

type PassHistory struct{ store PassHistoryRepository }

func NewPassHistory(store PassHistoryRepository) *PassHistory { return &PassHistory{store} }

func (s *PassHistory) ListPasses(ctx context.Context, actor string, f domain.PassFilter) (domain.PassPage, error) {
	if strings.TrimSpace(actor) == "" {
		return domain.PassPage{}, domain.ErrForbidden
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 1000000 ||
		(f.Type != "" && f.Type != "leave" && f.Type != "late_access") ||
		(f.Status != "" && f.Status != "pending" && f.Status != "approved" && f.Status != "rejected" && f.Status != "cancelled") {
		return domain.PassPage{}, domain.ErrInvalid
	}
	return s.store.OwnPasses(ctx, actor, f)
}
