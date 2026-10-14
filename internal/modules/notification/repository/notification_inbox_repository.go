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
	return tx, id, role, nil
}

// Ownership and CURRENT scope apply equally to list/count/read. Role changes
// cannot reveal old disciplinary alerts or another student's complaint.
const visible = ` FROM hostelhive.notifications n
 JOIN hostelhive.complaint_updates e ON e.update_id=n.event_id
 JOIN hostelhive.complaints c ON c.complaint_id=e.complaint_id
 JOIN hostelhive.students st ON st.student_id=c.student_id
 WHERE n.recipient_id=$1 AND (
 (e.action='created' AND $2::text IN ('admin','warden','sub_warden') AND ($2::text!='sub_warden' OR c.category='maintenance'))
 OR (e.action='status' AND $2::text='student' AND st.user_id=$1))`
const projection = `n.notification_id::text,c.complaint_id::text,e.action AS kind,c.category,e.new_status AS status,
 CASE WHEN e.action='created' THEN 'New '||c.category||' complaint' ELSE 'Your complaint is '||replace(e.new_status,'_',' ') END AS title,
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
