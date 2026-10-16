package service

import (
	"context"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
)

type StudentHistoryRepository interface {
	OwnEvents(context.Context, string, dto.StudentHistoryFilter) (domain.StudentHistoryPage, error)
}
type StudentHistory struct {
	repo     StudentHistoryRepository
	location *time.Location
}

func NewStudentHistory(repo StudentHistoryRepository, location *time.Location) *StudentHistory {
	return &StudentHistory{repo: repo, location: location}
}
func (s *StudentHistory) Events(ctx context.Context, actor string, q dto.StudentHistoryQuery) (domain.StudentHistoryPage, error) {
	if actor == "" {
		return domain.StudentHistoryPage{}, domain.ErrForbidden
	}
	if s.location == nil || q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 1000000 || (q.Direction != "" && q.Direction != "IN" && q.Direction != "OUT") {
		return domain.StudentHistoryPage{}, domain.ErrInvalid
	}
	f := dto.StudentHistoryFilter{StudentHistoryQuery: q, Timezone: s.location.String()}
	if q.From != "" || q.To != "" {
		from, e1 := time.Parse("2006-01-02", q.From)
		to, e2 := time.Parse("2006-01-02", q.To)
		if e1 != nil || e2 != nil || len(q.From) != 10 || len(q.To) != 10 || from.Year() < 1 || to.Year() > 9998 || to.Before(from) || to.After(from.AddDate(0, 0, 365)) {
			return domain.StudentHistoryPage{}, domain.ErrInvalid
		}
		start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, s.location)
		end := time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, s.location)
		f.Start, f.End = &start, &end
	}
	return s.repo.OwnEvents(ctx, actor, f)
}
