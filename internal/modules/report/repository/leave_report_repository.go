package repository

import (
	"context"
	"encoding/json"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Leaves struct{ pool *pgxpool.Pool }

func NewLeaves(pool *pgxpool.Pool) *Leaves { return &Leaves{pool} }
func (s *Leaves) Requests(ctx context.Context, actor string, f dto.LeaveFilter) (domain.LeavePage, error) {
	p := domain.LeavePage{From: f.From, To: f.To, Timezone: f.Timezone, StudentID: f.StudentID, Status: f.Status, Limit: f.Limit, Offset: f.Offset, Items: []domain.LeaveRequest{}}
	tx, err := beginReport(ctx, s.pool, actor)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	// Submission dates choose requests; current status filters the selected set.
	// Rows, full-period totals and empty-page counts share ONE statement snapshot.
	const sql = `WITH matching AS MATERIALIZED(
 SELECT leave_id::text,request_id::text,student_id::text,submitted_by::text,
 starts_at,ends_at,reason,status,created_at,decided_at,decided_by::text,decision_note
 FROM hostelhive.leave_requests
 WHERE created_at >= $1 AND created_at < $2
 AND ($3::uuid IS NULL OR student_id=$3) AND ($4::text IS NULL OR status=$4)
 ), summary AS (
 SELECT count(*) AS total,count(*) FILTER(WHERE status='pending') AS pending,
 count(*) FILTER(WHERE status='approved') AS approved,count(*) FILTER(WHERE status='rejected') AS rejected FROM matching
 ), page AS (
 SELECT * FROM matching ORDER BY created_at DESC,leave_id DESC LIMIT $5 OFFSET $6
 )
 SELECT s.total,s.pending,s.approved,s.rejected,
 COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.created_at DESC,p.leave_id DESC) FROM page p),'[]'::jsonb)
 FROM summary s`
	if err = tx.QueryRow(ctx, sql, f.Start, f.End, optional(f.StudentID), optional(f.Status), f.Limit, f.Offset).Scan(&p.Total, &p.Summary.Pending, &p.Summary.Approved, &p.Summary.Rejected, &raw); err != nil {
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
