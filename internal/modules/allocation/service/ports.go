package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
)

type Repository interface {
	List(context.Context, string, domain.Filter) (domain.Page, error)
	Assign(context.Context, string, dto.Assign) (domain.Allocation, error)
	Transfer(context.Context, string, string, dto.Transfer) (domain.Allocation, error)
	Revoke(context.Context, string, string) (domain.Allocation, error)
}
