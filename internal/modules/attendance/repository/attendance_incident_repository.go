package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	"github.com/jackc/pgx/v5"
)

const incidentColumns = `incident_id::text,device_id::text,event_id::text,student_id::text,direction,incident_type,occurred_at,hostel_timezone,exit_opening_minute,entry_deadline_minute,permission_window_seconds`

func (s *Devices) beginIncidentRead(ctx context.Context, actor string) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE MODE`); err != nil {
		rollback(tx)
		return nil, domain.ErrUnavailable
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden'))`, actor).Scan(&allowed); err != nil {
		rollback(tx)
		return nil, domain.ErrUnavailable
	}
	if !allowed {
		rollback(tx)
		return nil, domain.ErrForbidden
	}
	return tx, nil
}
func nullableIncidentFilter(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (s *Devices) Incidents(ctx context.Context, actor string, f dto.IncidentFilter) (domain.IncidentPage, error) {
	p := domain.IncidentPage{Items: []domain.Incident{}, Limit: f.Limit, Offset: f.Offset}
	tx, err := s.beginIncidentRead(ctx, actor)
	if err != nil {
		return p, err
	}
	defer rollback(tx)
	var raw []byte
	sql := `WITH matching AS (SELECT ` + incidentColumns + ` FROM hostelhive.attendance_incidents
 WHERE ($1::uuid IS NULL OR student_id=$1) AND ($2::text IS NULL OR direction=$2) AND ($3::text IS NULL OR incident_type=$3)),
 page AS (SELECT * FROM matching ORDER BY occurred_at DESC,incident_id DESC LIMIT $4 OFFSET $5)
 SELECT (SELECT count(*) FROM matching),COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.occurred_at DESC,p.incident_id DESC) FROM page p),'[]'::jsonb)`
	if err = tx.QueryRow(ctx, sql, nullableIncidentFilter(f.StudentID), nullableIncidentFilter(f.Direction), nullableIncidentFilter(f.IncidentType), f.Limit, f.Offset).Scan(&p.Total, &raw); err != nil {
		return p, domain.ErrUnavailable
	}
	if json.Unmarshal(raw, &p.Items) != nil {
		return p, domain.ErrUnavailable
	}
	p.HasMore = int64(p.Offset)+int64(len(p.Items)) < p.Total
	if tx.Commit(ctx) != nil {
		return p, domain.ErrUnavailable
	}
	return p, nil
}
func (s *Devices) Incident(ctx context.Context, actor, id string) (domain.Incident, error) {
	var v domain.Incident
	tx, err := s.beginIncidentRead(ctx, actor)
	if err != nil {
		return v, err
	}
	defer rollback(tx)
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT to_jsonb(v) FROM (SELECT `+incidentColumns+` FROM hostelhive.attendance_incidents WHERE incident_id=$1) v`, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, domain.ErrNotFound
	}
	if err != nil || json.Unmarshal(raw, &v) != nil {
		return v, domain.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return v, domain.ErrUnavailable
	}
	return v, nil
}
