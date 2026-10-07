package service

import (
	"context"

	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	sdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
)

type Repository interface {
	List(context.Context, sdomain.Filter) (sdomain.Page, error)
	Get(context.Context, string) (sdomain.Profile, error)
	Create(context.Context, string, sdto.CreateInput) (sdomain.Profile, error)
	Update(context.Context, string, string, sdto.Details) (sdomain.Profile, error)
	Delete(context.Context, string, string) error
}
