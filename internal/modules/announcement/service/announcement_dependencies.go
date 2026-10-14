package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/dto"
)

type Repository interface {
	Publish(context.Context, string, dto.Publish) (domain.Publication, error)
	List(context.Context, string, dto.Filter) (domain.Page, error)
	Get(context.Context, string, string) (domain.Announcement, error)
	Summary(context.Context, string, string) (domain.DeliverySummary, error)
}
