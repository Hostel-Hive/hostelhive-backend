package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Store { return &Store{p} }
func rollback(tx pgx.Tx) {
	ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
	defer c()
	_ = tx.Rollback(ctx)
}
func (s *Store) begin(ctx context.Context, uid string) (pgx.Tx, string, string, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, "", "", domain.ErrUnavailable
	}
	if _, e = tx.Exec(ctx, `LOCK TABLE hostelhive.users IN SHARE MODE`); e != nil {
		rollback(tx)
		return nil, "", "", domain.ErrUnavailable
	}
	var id, role string
	e = tx.QueryRow(ctx, `SELECT user_id::text,role FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden','sub_warden','student')`, uid).Scan(&id, &role)
	if e != nil {
		rollback(tx)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, "", "", domain.ErrForbidden
		}
		return nil, "", "", domain.ErrUnavailable
	}
	if role == "student" {
		// Preserve historical complaint/leave access, but stabilize membership while
		// evaluating announcement eligibility against concurrent profile archival.
		var sid string
		e = tx.QueryRow(ctx, `SELECT student_id::text FROM hostelhive.students WHERE user_id=$1 FOR SHARE`, id).Scan(&sid)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			rollback(tx)
			return nil, "", "", domain.ErrUnavailable
		}
	}
	return tx, id, role, nil
}

// Ownership and CURRENT scope apply equally to list/count/read. Event content is
// a snapshot; opening the underlying complaint/leave uses its own authorised API.
const visible = ` FROM hostelhive.notifications n
 JOIN hostelhive.notification_events e ON e.event_id=n.event_id
 LEFT JOIN hostelhive.complaint_updates ce ON ce.update_id=e.complaint_update_id
 LEFT JOIN hostelhive.complaints c ON c.complaint_id=ce.complaint_id
 LEFT JOIN hostelhive.leave_requests l ON l.leave_id=e.leave_id
 LEFT JOIN hostelhive.announcements a ON a.announcement_id=e.announcement_id
 LEFT JOIN hostelhive.students st ON st.student_id=COALESCE(c.student_id,l.student_id)
 WHERE n.recipient_id=$1 AND (
 (e.source_type='complaint' AND (
  (e.kind='created' AND $2::text IN ('admin','warden','sub_warden') AND ($2::text!='sub_warden' OR c.category='maintenance'))
  OR (e.kind='status' AND $2::text='student' AND st.user_id=$1)))
 OR (e.source_type='leave' AND (
  (e.kind='leave_submitted' AND $2::text IN ('admin','warden'))
  OR (e.kind='leave_decided' AND $2::text='student' AND st.user_id=$1)))
 OR (e.source_type='announcement' AND $2::text='student' AND a.audience='all_students'
 AND EXISTS(SELECT 1 FROM hostelhive.students member WHERE member.user_id=$1 AND member.deleted_at IS NULL)))`
const projection = `n.notification_id::text,e.source_type,COALESCE(c.complaint_id::text,'') AS complaint_id,
 COALESCE(l.leave_id::text,'') AS leave_id,COALESCE(a.announcement_id::text,'') AS announcement_id,e.kind,COALESCE(c.category,'') AS category,e.status,
 CASE e.kind
 WHEN 'created' THEN 'New '||c.category||' complaint'
 WHEN 'status' THEN 'Your complaint is '||replace(e.status,'_',' ')
 WHEN 'leave_submitted' THEN 'New leave request'
 WHEN 'leave_decided' THEN 'Your leave request is '||e.status
 WHEN 'announcement_published' THEN a.title END AS title,
 e.occurred_at,n.created_at,n.read_at`

func (s *Store) List(ctx context.Context, uid string, f dto.Filter) (domain.Page, error) {
	p := domain.Page{Items: []domain.Notification{}, Limit: f.Limit, Offset: f.Offset}
	tx, id, role, e := s.begin(ctx, uid)
	if e != nil {
		return p, e
	}
	defer rollback(tx)
	var raw []byte
	sql := `WITH matching AS (SELECT ` + projection + visible + ` AND (NOT $3::boolean OR n.read_at IS NULL)), page AS (SELECT * FROM matching ORDER BY occurred_at DESC,notification_id DESC LIMIT $4 OFFSET $5)
 SELECT (SELECT count(*) FROM matching),COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.occurred_at DESC,p.notification_id DESC) FROM page p),'[]'::jsonb)`
	if e = tx.QueryRow(ctx, sql, id, role, f.UnreadOnly, f.Limit, f.Offset).Scan(&p.Total, &raw); e != nil {
		return p, domain.ErrUnavailable
	}
	if json.Unmarshal(raw, &p.Items) != nil {
		return p, domain.ErrUnavailable
	}
	p.HasMore = int64(f.Offset)+int64(len(p.Items)) < p.Total
	if tx.Commit(ctx) != nil {
		return p, domain.ErrUnavailable
	}
	return p, nil
}
func (s *Store) Unread(ctx context.Context, uid string) (domain.Unread, error) {
	tx, id, role, e := s.begin(ctx, uid)
	if e != nil {
		return domain.Unread{}, e
	}
	defer rollback(tx)
	var v domain.Unread
	if tx.QueryRow(ctx, `SELECT count(*)`+visible+` AND n.read_at IS NULL`, id, role).Scan(&v.Unread) != nil {
		return v, domain.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return v, domain.ErrUnavailable
	}
	return v, nil
}
func (s *Store) MarkRead(ctx context.Context, uid, nid string) (domain.Notification, error) {
	tx, id, role, e := s.begin(ctx, uid)
	if e != nil {
		return domain.Notification{}, e
	}
	defer rollback(tx)
	var v domain.Notification
	var raw []byte
	sql := `WITH allowed AS (SELECT n.notification_id` + visible + ` AND n.notification_id=$3 FOR UPDATE OF n)
 UPDATE hostelhive.notifications n SET read_at=COALESCE(n.read_at,clock_timestamp()) FROM allowed a
 WHERE n.notification_id=a.notification_id RETURNING n.notification_id::text`
	var found string
	e = tx.QueryRow(ctx, sql, id, role, nid).Scan(&found)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, domain.ErrNotFound
	}
	if e != nil {
		return v, domain.ErrUnavailable
	}
	if tx.QueryRow(ctx, `SELECT to_jsonb(v) FROM (SELECT `+projection+visible+` AND n.notification_id=$3) v`, id, role, nid).Scan(&raw) != nil {
		return v, domain.ErrUnavailable
	}
	if json.Unmarshal(raw, &v) != nil {
		return v, domain.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return v, domain.ErrUnavailable
	}
	return v, nil
}
