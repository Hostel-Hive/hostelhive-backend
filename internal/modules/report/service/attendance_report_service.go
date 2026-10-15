package service

import (
	"context"
	"strings"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/google/uuid"
)

type Attendance struct {
	store    AttendanceRepository
	location *time.Location
}

func NewAttendance(store AttendanceRepository, location *time.Location) *Attendance {
	return &Attendance{store: store, location: location}
}
func (s *Attendance) filter(q dto.AttendanceQuery) (dto.AttendanceFilter, error) {
	f := dto.AttendanceFilter{AttendanceQuery: q}
	if s.location == nil || q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 1000000 ||
		(q.Direction != "" && q.Direction != "IN" && q.Direction != "OUT") {
		return f, domain.ErrInvalid
	}
	if q.StudentID != "" {
		id, err := uuid.Parse(q.StudentID)
		if err != nil || id.String() != q.StudentID || id == uuid.Nil {
			return f, domain.ErrInvalid
		}
	}
	// Parse as UTC calendar dates to count days independently of daylight saving.
	from, e1 := time.Parse("2006-01-02", q.From)
	to, e2 := time.Parse("2006-01-02", q.To)
	if e1 != nil || e2 != nil || len(q.From) != 10 || len(q.To) != 10 ||
		from.Year() < 1 || to.Year() > 9998 || to.Before(from) {
		return f, domain.ErrInvalid
	}
	if to.After(from.AddDate(0, 0, 365)) {
		return f, domain.ErrInvalid
	}
	// Bound the calendar range before converting it to a time.Duration.
	f.Days = int(to.Sub(from).Hours()/24) + 1
	f.Start = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, s.location)
	f.End = time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, s.location)
	f.Timezone = s.location.String()
	if strings.TrimSpace(f.Timezone) == "" {
		return f, domain.ErrInvalid
	}
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
