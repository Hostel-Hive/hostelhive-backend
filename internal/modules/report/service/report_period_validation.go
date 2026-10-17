package service

import (
	"strings"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/google/uuid"
)

type reportPeriod struct {
	start, end time.Time
	timezone   string
	days       int
}

// Shared calendar/profile/paging rules for reports. Count calendar dates in UTC
// before constructing hostel-local midnights; a local day need not last 24 hours.
func validatePeriod(fromDate, toDate, student string, limit, offset int, loc *time.Location) (reportPeriod, error) {
	var p reportPeriod
	if loc == nil || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return p, domain.ErrInvalid
	}
	if student != "" {
		id, err := uuid.Parse(student)
		if err != nil || id.String() != student || id == uuid.Nil {
			return p, domain.ErrInvalid
		}
	}
	from, e1 := time.Parse("2006-01-02", fromDate)
	to, e2 := time.Parse("2006-01-02", toDate)
	if e1 != nil || e2 != nil || len(fromDate) != 10 || len(toDate) != 10 ||
		from.Year() < 1 || to.Year() > 9998 || to.Before(from) || to.After(from.AddDate(0, 0, 365)) {
		return p, domain.ErrInvalid
	}
	p.days = int(to.Sub(from).Hours()/24) + 1
	p.start = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	p.end = time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, loc)
	p.timezone = loc.String()
	if strings.TrimSpace(p.timezone) == "" {
		return p, domain.ErrInvalid
	}
	return p, nil
}
