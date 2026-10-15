package repository

import (
	"context"
	"encoding/json"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Complaints struct{ pool *pgxpool.Pool }

func NewComplaints(pool *pgxpool.Pool) *Complaints { return &Complaints{pool} }
func (s *Complaints) Records(ctx context.Context, actor string, f dto.ComplaintFilter) (domain.ComplaintPage, error) {
	p := domain.ComplaintPage{From: f.From, To: f.To, Timezone: f.Timezone, StudentID: f.StudentID, Category: f.Category, Status: f.Status, Limit: f.Limit, Offset: f.Offset, Items: []domain.ComplaintRecord{}}
	tx, err := beginReport(ctx, s.pool, actor)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	// Count complaints before joining history. A complaint with many updates counts once.
	// Resolve only page items and only their CURRENT resolved state, using audit versions.
	// Full-period totals and rows share a single SQL statement snapshot.
	const sql = `WITH matching AS MATERIALIZED (
 SELECT complaint_id::text,request_id::text,student_id::text,submitted_by::text,
 category,description,status,version,created_at,updated_at
 FROM hostelhive.complaints
 WHERE created_at >= $1 AND created_at < $2
 AND ($3::uuid IS NULL OR student_id=$3) AND ($4::text IS NULL OR category=$4)
 AND ($5::text IS NULL OR status=$5)
 ), summary AS (
 SELECT count(*) AS total,count(*) FILTER(WHERE status='pending') AS pending,
 count(*) FILTER(WHERE status='in_progress') AS in_progress,count(*) FILTER(WHERE status='resolved') AS resolved,
 count(*) FILTER(WHERE category='maintenance') AS maintenance,count(*) FILTER(WHERE category='disciplinary') AS disciplinary
 FROM matching
 ), page AS (
 SELECT * FROM matching ORDER BY created_at DESC,complaint_id DESC LIMIT $6 OFFSET $7
 ), records AS (
 SELECT p.*,CASE WHEN r.update_id IS NULL OR r.new_status<>'resolved' THEN NULL ELSE jsonb_build_object(
 'update_id',r.update_id,'resolved_at',r.occurred_at,'resolved_by',r.actor_id,'note',r.text) END AS resolution
 FROM page p LEFT JOIN LATERAL (
 SELECT u.update_id,u.occurred_at,u.actor_id,u.text,u.new_status FROM hostelhive.complaint_updates u
 WHERE u.complaint_id=p.complaint_id::uuid AND p.status='resolved'
 AND u.action='status' AND u.version<=p.version
 ORDER BY u.version DESC LIMIT 1
 ) r ON true
 )
 SELECT s.total,s.pending,s.in_progress,s.resolved,s.maintenance,s.disciplinary,
 COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.created_at DESC,r.complaint_id DESC) FROM records r),'[]'::jsonb)
 FROM summary s`
	var raw []byte
	statuses, categories := &p.Summary.ByStatus, &p.Summary.ByCategory
	if err = tx.QueryRow(ctx, sql, f.Start, f.End, optional(f.StudentID), optional(f.Category), optional(f.Status), f.Limit, f.Offset).Scan(&p.Total, &statuses.Pending, &statuses.InProgress, &statuses.Resolved, &categories.Maintenance, &categories.Disciplinary, &raw); err != nil {
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
