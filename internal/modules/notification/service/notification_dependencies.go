package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
)

type Repository interface {
	List(context.Context, string, dto.Filter) (domain.Page, error)
	Unread(context.Context, string) (domain.Unread, error)
	MarkRead(context.Context, string, string) (domain.Notification, error)
}
