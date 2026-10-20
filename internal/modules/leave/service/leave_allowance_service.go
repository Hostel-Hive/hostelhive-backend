package service

import (
	"context"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
)

const AllowanceTimezone = "Asia/Colombo"
const YearlyAllowanceDays = 18

type AllowanceRepository interface {
	AllowanceUsage(context.Context, string, domain.AllowancePeriod) (domain.AllowanceUsage, error)
}

type AllowanceReader interface {
	GetAllowance(context.Context, string, int) (domain.AllowanceSummary, error)
}

type Allowances struct {
	store    AllowanceRepository
	location *time.Location
	now      func() time.Time
}

func NewAllowance(store AllowanceRepository) *Allowances {
	location, _ := time.LoadLocation(AllowanceTimezone)
	return &Allowances{store: store, location: location, now: time.Now}
}

// Year zero means the current Colombo calendar year, never the client's timezone.
func (s *Allowances) GetAllowance(ctx context.Context, actor string, year int) (domain.AllowanceSummary, error) {
	if strings.TrimSpace(actor) == "" {
		return domain.AllowanceSummary{}, domain.ErrForbidden
	}
	if s.location == nil {
		return domain.AllowanceSummary{}, domain.ErrUnavailable
	}
	if year == 0 {
		year = s.now().In(s.location).Year()
	}
	if year < 1 || year > 9998 {
		return domain.AllowanceSummary{}, domain.ErrInvalid
	}
	period := domain.AllowancePeriod{
		Year: year, Timezone: AllowanceTimezone,
		Start: time.Date(year, 1, 1, 0, 0, 0, 0, s.location),
		End:   time.Date(year+1, 1, 1, 0, 0, 0, 0, s.location),
	}
	usage, err := s.store.AllowanceUsage(ctx, actor, period)
	if err != nil {
		return domain.AllowanceSummary{}, err
	}
	return domain.AllowanceSummary{
		StudentID: usage.StudentID, Year: year, Timezone: AllowanceTimezone,
		TotalDays: YearlyAllowanceDays, UsedDays: usage.UsedDays,
		RemainingDays: max(YearlyAllowanceDays-usage.UsedDays, 0),
		ExceededDays:  max(usage.UsedDays-YearlyAllowanceDays, 0),
	}, nil
}
