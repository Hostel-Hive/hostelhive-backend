package service

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
)

type Repository interface {
	List(context.Context, int, int) (domain.Page, error)
	Get(context.Context, string) (domain.Profile, error)
	Put(context.Context, string, string, dto.Details) (domain.Profile, error)
	PutSelf(context.Context, string, dto.SelfDetails) (domain.Profile, error)
}
