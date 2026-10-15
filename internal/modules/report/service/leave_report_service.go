package service

import (
	"context"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
)

type Leaves struct {
	store    LeaveRepository
	location *time.Location
}

func NewLeaves(store LeaveRepository, location *time.Location) *Leaves {
	return &Leaves{store, location}
}
func (s *Leaves) Requests(ctx context.Context, actor string, q dto.LeaveQuery) (domain.LeavePage, error) {
	if q.Status != "" && q.Status != "pending" && q.Status != "approved" && q.Status != "rejected" {
		return domain.LeavePage{}, domain.ErrInvalid
	}
	p, err := validatePeriod(q.From, q.To, q.StudentID, q.Limit, q.Offset, s.location)
	if err != nil {
		return domain.LeavePage{}, err
	}
	return s.store.Requests(ctx, actor, dto.LeaveFilter{LeaveQuery: q, Start: p.start, End: p.end, Timezone: p.timezone})
}
