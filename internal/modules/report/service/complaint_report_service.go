package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"time"
)

type Complaints struct {
	store    ComplaintRepository
	location *time.Location
}

func NewComplaints(store ComplaintRepository, location *time.Location) *Complaints {
	return &Complaints{store, location}
}
func (s *Complaints) filter(q dto.ComplaintQuery) (dto.ComplaintFilter, error) {
	f := dto.ComplaintFilter{ComplaintQuery: q}
	if (q.Category != "" && q.Category != "maintenance" && q.Category != "disciplinary") || (q.Status != "" && q.Status != "pending" && q.Status != "in_progress" && q.Status != "resolved") {
		return f, domain.ErrInvalid
	}
	p, err := validatePeriod(q.From, q.To, q.StudentID, q.Limit, q.Offset, s.location)
	if err != nil {
		return f, err
	}
	f.Start, f.End, f.Timezone = p.start, p.end, p.timezone
	return f, nil
}
func (s *Complaints) Records(ctx context.Context, actor string, q dto.ComplaintQuery) (domain.ComplaintPage, error) {
	f, err := s.filter(q)
	if err != nil {
		return domain.ComplaintPage{}, err
	}
	return s.store.Records(ctx, actor, f)
}
