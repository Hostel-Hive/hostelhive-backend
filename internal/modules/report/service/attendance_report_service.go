package service

import (
	"context"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
)

type Attendance struct {
	store    AttendanceRepository
	location *time.Location
}

func NewAttendance(store AttendanceRepository, location *time.Location) *Attendance {
	return &Attendance{store, location}
}
func (s *Attendance) filter(q dto.AttendanceQuery) (dto.AttendanceFilter, error) {
	f := dto.AttendanceFilter{AttendanceQuery: q}
	if q.Direction != "" && q.Direction != "IN" && q.Direction != "OUT" {
		return f, domain.ErrInvalid
	}
	p, err := validatePeriod(q.From, q.To, q.StudentID, q.Limit, q.Offset, s.location)
	if err != nil {
		return f, err
	}
	f.Start, f.End, f.Timezone, f.Days = p.start, p.end, p.timezone, p.days
	return f, nil
}
func (s *Attendance) Movements(ctx context.Context, actor string, q dto.AttendanceQuery) (domain.MovementPage, error) {
	f, err := s.filter(q)
	if err != nil {
		return domain.MovementPage{}, err
	}
	return s.store.Movements(ctx, actor, f)
}
func (s *Attendance) Daily(ctx context.Context, actor string, q dto.AttendanceQuery) (domain.DailyPage, error) {
	f, err := s.filter(q)
	if err != nil {
		return domain.DailyPage{}, err
	}
	return s.store.Daily(ctx, actor, f)
}
