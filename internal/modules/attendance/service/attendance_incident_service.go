package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
)

type IncidentRepository interface {
	Incidents(context.Context, string, dto.IncidentFilter) (domain.IncidentPage, error)
	Incident(context.Context, string, string) (domain.Incident, error)
}
type Incidents struct{ store IncidentRepository }

func NewIncidents(store IncidentRepository) *Incidents { return &Incidents{store} }
func (s *Incidents) Incidents(ctx context.Context, actor string, f dto.IncidentFilter) (domain.IncidentPage, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 1000000 || (f.StudentID != "" && !canonicalID(f.StudentID)) || (f.Direction != "" && f.Direction != "IN" && f.Direction != "OUT") || (f.IncidentType != "" && f.IncidentType != "late_entry" && f.IncidentType != "early_exit" && f.IncidentType != "late_exit") {
		return domain.IncidentPage{}, domain.ErrInvalid
	}
	return s.store.Incidents(ctx, actor, f)
}
func (s *Incidents) Incident(ctx context.Context, actor, id string) (domain.Incident, error) {
	if !canonicalID(id) {
		return domain.Incident{}, domain.ErrInvalid
	}
	return s.store.Incident(ctx, actor, id)
}
