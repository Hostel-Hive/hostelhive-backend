package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
)

type Repository interface {
	Submit(context.Context, string, dto.Submit) (domain.Request, error)
	Decide(context.Context, string, string, dto.Decision) (domain.Request, error)
	Get(context.Context, string, string) (domain.Request, error)
	List(context.Context, string, domain.Filter) (domain.Page, error)
}
