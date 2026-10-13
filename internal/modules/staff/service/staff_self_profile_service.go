package service

import (
	"context"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

func (s *Service) PutSelf(ctx context.Context, actor string, d dto.SelfDetails) (domain.Profile, error) {
	d.FullName = strings.TrimSpace(d.FullName)
	if strings.TrimSpace(actor) == "" || !validation.Text(d.FullName, 200) {
		return domain.Profile{}, domain.ErrInvalid
	}
	return s.repo.PutSelf(ctx, actor, d)
}
