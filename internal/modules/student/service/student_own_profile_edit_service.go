package service

import (
	"context"
	"strings"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type OwnProfileEditRepository interface {
	PatchOwnProfile(context.Context, string, dto.OwnProfilePatch) (d.OwnProfile, error)
}

type OwnProfileEditor struct{ repo OwnProfileEditRepository }

func NewOwnProfileEditor(repo OwnProfileEditRepository) *OwnProfileEditor {
	return &OwnProfileEditor{repo: repo}
}

func (s *OwnProfileEditor) Patch(ctx context.Context, actor string, input dto.OwnProfilePatch) (d.OwnProfile, error) {
	if strings.TrimSpace(actor) == "" {
		return d.OwnProfile{}, d.ErrForbidden
	}
	if input.ContactPhone == nil && input.Guardians == nil {
		return d.OwnProfile{}, d.ErrInvalid
	}
	if input.ContactPhone != nil {
		phone := strings.TrimSpace(*input.ContactPhone)
		if !validation.Phone(phone) {
			return d.OwnProfile{}, d.ErrInvalid
		}
		input.ContactPhone = &phone
	}
	if input.Guardians != nil {
		gs, err := normalizeGuardians(*input.Guardians)
		if err != nil {
			return d.OwnProfile{}, err
		}
		input.Guardians = &gs
	}
	return s.repo.PatchOwnProfile(ctx, actor, input)
}
