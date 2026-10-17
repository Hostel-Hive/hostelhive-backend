package repository

import (
	"context"
	"encoding/json"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Attendance struct{ pool *pgxpool.Pool }

func NewAttendance(pool *pgxpool.Pool) *Attendance { return &Attendance{pool} }

const matching = `WITH matching AS MATERIALIZED (
 SELECT e.event_id::text,e.device_id::text,e.student_id::text,e.direction,e.occurred_at,
 e.access_result,e.late_access_id::text,
 (SELECT i.incident_id::text FROM hostelhive.attendance_incidents i WHERE i.device_id=e.device_id AND i.event_id=e.event_id) AS incident_id
 FROM hostelhive.attendance_events e
 WHERE e.outcome='recorded' AND e.occurred_at >= $1 AND e.occurred_at < $2
 AND ($3::uuid IS NULL OR e.student_id=$3) AND ($4::text IS NULL OR e.direction=$4)
)`
const totals = `(SELECT count(*) FILTER(WHERE direction='IN') FROM matching),
 (SELECT count(*) FILTER(WHERE direction='OUT') FROM matching),
 (SELECT count(DISTINCT student_id) FROM matching)`

func optional(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func metadata(f dto.AttendanceFilter) domain.Metadata {
	return domain.Metadata{From: f.From, To: f.To, Timezone: f.Timezone, StudentID: f.StudentID, Direction: f.Direction, Limit: f.Limit, Offset: f.Offset}
}
func (s *Attendance) Movements(ctx context.Context, actor string, f dto.AttendanceFilter) (domain.MovementPage, error) {
	p := domain.MovementPage{Metadata: metadata(f), Items: []domain.Movement{}}
	tx, err := beginReport(ctx, s.pool, actor)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	sql := matching + `, page AS (
 SELECT * FROM matching ORDER BY occurred_at DESC,device_id,event_id LIMIT $5 OFFSET $6)
 SELECT (SELECT count(*) FROM matching),` + totals + `,
 COALESCE((SELECT jsonb_agg(jsonb_strip_nulls(to_jsonb(p)) ORDER BY p.occurred_at DESC,p.device_id,p.event_id) FROM page p),'[]'::jsonb)`
	if err = tx.QueryRow(ctx, sql, f.Start, f.End, optional(f.StudentID), optional(f.Direction), f.Limit, f.Offset).Scan(
		&p.Total, &p.Summary.Entries, &p.Summary.Exits, &p.Summary.DistinctStudents, &raw); err != nil {
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
func (s *Attendance) Daily(ctx context.Context, actor string, f dto.AttendanceFilter) (domain.DailyPage, error) {
	p := domain.DailyPage{Metadata: metadata(f), Items: []domain.Day{}}
	tx, err := beginReport(ctx, s.pool, actor)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	sql := matching + `, counts AS (
 SELECT timezone($5::text,occurred_at)::date AS day,
 count(*) FILTER(WHERE direction='IN') AS entries,
 count(*) FILTER(WHERE direction='OUT') AS exits,
 count(DISTINCT student_id) AS distinct_students FROM matching GROUP BY 1
 ), days AS (
 SELECT $6::date + n AS day FROM generate_series(0,$7::int-1) n
 ), page AS (
 SELECT to_char(d.day,'YYYY-MM-DD') AS date,COALESCE(c.entries,0) AS entries,
 COALESCE(c.exits,0) AS exits,COALESCE(c.distinct_students,0) AS distinct_students
 FROM days d LEFT JOIN counts c USING(day) ORDER BY d.day DESC LIMIT $8 OFFSET $9
 )
 SELECT $7::bigint,` + totals + `,
 COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.date DESC) FROM page p),'[]'::jsonb)`
	if err = tx.QueryRow(ctx, sql, f.Start, f.End, optional(f.StudentID), optional(f.Direction), f.Timezone, f.From, f.Days, f.Limit, f.Offset).Scan(
		&p.Total, &p.Summary.Entries, &p.Summary.Exits, &p.Summary.DistinctStudents, &raw); err != nil {
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
