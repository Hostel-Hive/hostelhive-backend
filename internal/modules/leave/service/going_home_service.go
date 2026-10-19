package service

import (
	"context"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type GoingHomeRepository interface {
	DeclareGoingHome(context.Context, string, dto.GoingHomeInput) (domain.GoingHomeDeclaration, bool, error)
}

type GoingHome struct{ store GoingHomeRepository }

func NewGoingHome(store GoingHomeRepository) *GoingHome { return &GoingHome{store} }
func (s *GoingHome) DeclareGoingHome(ctx context.Context, actor string, in dto.GoingHomeInput) (domain.GoingHomeDeclaration, bool, error) {
	if strings.TrimSpace(actor) == "" {
		return domain.GoingHomeDeclaration{}, false, domain.ErrForbidden
	}
	if !validation.UUID(in.RequestID) || in.RequestID == "00000000-0000-0000-0000-000000000000" {
		return domain.GoingHomeDeclaration{}, false, domain.ErrInvalid
	}
	in.RequestID = strings.ToLower(in.RequestID)
	return s.store.DeclareGoingHome(ctx, actor, in)
}
