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

type LeaveRepository interface {
	Requests(context.Context, string, dto.LeaveFilter) (domain.LeavePage, error)
}
type LeaveReports interface {
	Requests(context.Context, string, dto.LeaveQuery) (domain.LeavePage, error)
}

type ComplaintRepository interface {
	Records(context.Context, string, dto.ComplaintFilter) (domain.ComplaintPage, error)
}
type ComplaintReports interface {
	Records(context.Context, string, dto.ComplaintQuery) (domain.ComplaintPage, error)
}

type ExcelWorkbookWriter interface {
	Build(context.Context, domain.ExportWorkbook) ([]byte, error)
}
type ExcelReports interface {
	Export(context.Context, string, domain.ReportKind, dto.ExportQuery) (domain.ExportFile, error)
}
