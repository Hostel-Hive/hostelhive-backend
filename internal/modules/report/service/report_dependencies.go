package service

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
)

type AttendanceRepository interface {
	Movements(context.Context, string, dto.AttendanceFilter) (domain.MovementPage, error)
	Daily(context.Context, string, dto.AttendanceFilter) (domain.DailyPage, error)
}
type AttendanceReports interface {
	Movements(context.Context, string, dto.AttendanceQuery) (domain.MovementPage, error)
	Daily(context.Context, string, dto.AttendanceQuery) (domain.DailyPage, error)
}
