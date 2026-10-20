package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/dto"
)

type Repository interface {
	Cancel(context.Context, string, string) (domain.Request, error)
	Submit(context.Context, string, dto.Submit) (domain.Request, error)
	Decide(context.Context, string, string, dto.Decision) (domain.Request, error)
	Get(context.Context, string, string) (domain.Request, error)
	List(context.Context, string, domain.Filter) (domain.Page, error)
}
