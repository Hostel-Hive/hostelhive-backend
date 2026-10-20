package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/jackc/pgx/v5"
)

// OwnPasses reads both source tables without invoking either write workflow.
// Authorization, profile, count and page share one read-only MVCC snapshot.
func (s *Store) OwnPasses(ctx context.Context, uid string, f domain.PassFilter) (domain.PassPage, error) {
	page := domain.PassPage{Items: []domain.PassItem{}, Limit: f.Limit, Offset: f.Offset}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, domain.ErrUnavailable
	}
	defer rollback(tx)
	var user, role string
	var active, setup bool
	err = tx.QueryRow(ctx, `SELECT user_id::text,role,is_active,
 EXISTS(SELECT 1 FROM hostelhive.student_invitations i WHERE i.user_id=u.user_id AND i.setup_completed_at IS NULL)
 FROM hostelhive.users u WHERE firebase_uid=$1`, uid).Scan(&user, &role, &active, &setup)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (role != "student" || !active || setup)) {
		return page, domain.ErrForbidden
	}
	if err != nil {
		return page, domain.ErrUnavailable
	}
	var student string
	err = tx.QueryRow(ctx, `SELECT student_id::text FROM hostelhive.students WHERE user_id=$1 AND deleted_at IS NULL`, user).Scan(&student)
	if errors.Is(err, pgx.ErrNoRows) {
		return page, domain.ErrIneligible
	}
	if err != nil {
		return page, domain.ErrUnavailable
	}
	// UNION ALL preserves distinct requests, including colliding UUIDs across
	// tables. Type and source_id make the timestamp tie-breaker a total order.
	const query = `WITH sources AS (
 SELECT 'leave'::text AS type,leave_id AS source_id,request_id,student_id,reason,status,created_at,
 starts_at,ends_at,NULL::text AS direction,NULL::timestamptz AS expected_at,
 decided_at,decided_by,decision_note,cancelled_at,cancelled_by,NULL::timestamptz AS consumed_at
 FROM hostelhive.leave_requests WHERE student_id=$1
 UNION ALL
 SELECT 'late_access',late_access_id,request_id,student_id,reason,status,created_at,
 NULL::timestamptz,NULL::timestamptz,direction,expected_at,
 decided_at,decided_by,decision_note,cancelled_at,cancelled_by,consumed_at
 FROM hostelhive.late_access_requests WHERE student_id=$1
 ), matching AS (
 SELECT * FROM sources WHERE ($2::text='' OR type=$2) AND ($3::text='' OR status=$3)
 ), page AS (
 SELECT * FROM matching ORDER BY created_at DESC,type COLLATE "C" ASC,source_id DESC LIMIT $4 OFFSET $5
 ) SELECT (SELECT count(*) FROM matching),
 COALESCE(jsonb_agg(to_jsonb(page) ORDER BY created_at DESC,type COLLATE "C" ASC,source_id DESC),'[]'::jsonb) FROM page`
	var payload []byte
	if err = tx.QueryRow(ctx, query, student, f.Type, f.Status, f.Limit, f.Offset).Scan(&page.Total, &payload); err != nil {
		return page, domain.ErrUnavailable
	}
	if json.Unmarshal(payload, &page.Items) != nil {
		return domain.PassPage{}, domain.ErrUnavailable
	}
	page.HasMore = int64(page.Offset)+int64(len(page.Items)) < page.Total
	if tx.Commit(ctx) != nil {
		return domain.PassPage{}, domain.ErrUnavailable
	}
	return page, nil
}
