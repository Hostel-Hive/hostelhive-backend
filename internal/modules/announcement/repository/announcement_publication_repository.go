package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/dto"
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
	e = tx.QueryRow(ctx, `SELECT user_id::text,role FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden','student')`, uid).Scan(&id, &role)
	if e != nil {
		rollback(tx)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, "", "", domain.ErrForbidden
		}
		return nil, "", "", domain.ErrUnavailable
	}
	if role == "student" {
		var sid string
		e = tx.QueryRow(ctx, `SELECT student_id::text FROM hostelhive.students WHERE user_id=$1 AND deleted_at IS NULL FOR SHARE`, id).Scan(&sid)
		if e != nil {
			rollback(tx)
			if errors.Is(e, pgx.ErrNoRows) {
				return nil, "", "", domain.ErrForbidden
			}
			return nil, "", "", domain.ErrUnavailable
		}
	}
	return tx, id, role, nil
}
func scan(row pgx.Row) (domain.Announcement, error) {
	var v domain.Announcement
	var raw []byte
	if e := row.Scan(&raw); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return v, domain.ErrNotFound
		}
		return v, domain.ErrUnavailable
	}
	if json.Unmarshal(raw, &v) != nil {
		return v, domain.ErrUnavailable
	}
	return v, nil
}
func (s *Store) Publish(ctx context.Context, uid string, in dto.Publish) (domain.Publication, error) {
	tx, id, role, e := s.begin(ctx, uid)
	if e != nil {
		return domain.Publication{}, e
	}
	defer rollback(tx)
	if role != "admin" && role != "warden" {
		return domain.Publication{}, domain.ErrForbidden
	}
	// Serialize publisher-scoped request identities and snapshot student eligibility.
	if _, e = tx.Exec(ctx, `SELECT user_id FROM hostelhive.users WHERE user_id=$1 FOR UPDATE`, id); e != nil {
		return domain.Publication{}, domain.ErrUnavailable
	}
	if _, e = tx.Exec(ctx, `LOCK TABLE hostelhive.students IN SHARE MODE`); e != nil {
		return domain.Publication{}, domain.ErrUnavailable
	}
	prior, e := scan(tx.QueryRow(ctx, `SELECT to_jsonb(a) FROM hostelhive.announcements a WHERE published_by=$1 AND request_id=$2`, id, in.RequestID))
	if e == nil {
		faculty := ""
		if prior.Faculty != nil {
			faculty = *prior.Faculty
		}
		if prior.Title != in.Title || prior.Content != in.Content || prior.Audience != in.Audience || faculty != in.Faculty {
			return domain.Publication{}, domain.ErrConflict
		}
		if tx.Commit(ctx) != nil {
			return domain.Publication{}, domain.ErrUnavailable
		}
		return domain.Publication{Announcement: prior, Replayed: true}, nil
	}
	if !errors.Is(e, domain.ErrNotFound) {
		return domain.Publication{}, e
	}
	if in.Audience == "faculty" {
		var known bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.students WHERE deleted_at IS NULL AND lower(btrim(faculty))=$1)`, in.Faculty).Scan(&known); e != nil {
			return domain.Publication{}, domain.ErrUnavailable
		}
		if !known {
			return domain.Publication{}, domain.ErrInvalid
		}
	}
	v, e := scan(tx.QueryRow(ctx, `INSERT INTO hostelhive.announcements(request_id,published_by,title,content,audience,faculty) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')) RETURNING to_jsonb(announcements)`, in.RequestID, id, in.Title, in.Content, in.Audience, in.Faculty))
	if e != nil {
		return domain.Publication{}, e
	}
	if tx.Commit(ctx) != nil {
		return domain.Publication{}, domain.ErrUnavailable
	}
	return domain.Publication{Announcement: v}, nil
}

// Staff can read all notices; Students must match the current audience. The
// Student profile was locked by begin, including against a concurrent transfer.
const audienceScope = `($2::text IN ('admin','warden') OR a.audience='all_students'
 OR (a.audience='faculty' AND EXISTS(SELECT 1 FROM hostelhive.students member
 WHERE member.user_id=$1 AND member.deleted_at IS NULL AND lower(btrim(member.faculty))=a.faculty)))`

func (s *Store) Get(ctx context.Context, uid, aid string) (domain.Announcement, error) {
	tx, id, role, e := s.begin(ctx, uid)
	if e != nil {
		return domain.Announcement{}, e
	}
	defer rollback(tx)
	v, e := scan(tx.QueryRow(ctx, `SELECT to_jsonb(a) FROM hostelhive.announcements a WHERE announcement_id=$3 AND `+audienceScope, id, role, aid))
	if e != nil {
		return v, e
	}
	if tx.Commit(ctx) != nil {
		return v, domain.ErrUnavailable
	}
	return v, nil
}
func (s *Store) List(ctx context.Context, uid string, f dto.Filter) (domain.Page, error) {
	p := domain.Page{Items: []domain.Announcement{}, Limit: f.Limit, Offset: f.Offset}
	tx, id, role, e := s.begin(ctx, uid)
	if e != nil {
		return p, e
	}
	defer rollback(tx)
	var raw []byte
	e = tx.QueryRow(ctx, `WITH matching AS(SELECT a.* FROM hostelhive.announcements a WHERE `+audienceScope+`),
 page AS(SELECT * FROM matching ORDER BY published_at DESC,announcement_id DESC LIMIT $3 OFFSET $4)
 SELECT (SELECT count(*) FROM matching),COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.published_at DESC,p.announcement_id DESC) FROM page p),'[]'::jsonb)`, id, role, f.Limit, f.Offset).Scan(&p.Total, &raw)
	if e != nil || json.Unmarshal(raw, &p.Items) != nil {
		return p, domain.ErrUnavailable
	}
	p.HasMore = int64(f.Offset)+int64(len(p.Items)) < p.Total
	if tx.Commit(ctx) != nil {
		return p, domain.ErrUnavailable
	}
	return p, nil
}
func (s *Store) Summary(ctx context.Context, uid, aid string) (domain.DeliverySummary, error) {
	tx, _, role, e := s.begin(ctx, uid)
	if e != nil {
		return domain.DeliverySummary{}, e
	}
	defer rollback(tx)
	if role != "admin" && role != "warden" {
		return domain.DeliverySummary{}, domain.ErrForbidden
	}
	if _, e = scan(tx.QueryRow(ctx, `SELECT to_jsonb(a) FROM hostelhive.announcements a WHERE announcement_id=$1`, aid)); e != nil {
		return domain.DeliverySummary{}, e
	}
	v := domain.DeliverySummary{AnnouncementID: aid}
	e = tx.QueryRow(ctx, `SELECT count(o.recipient_id),count(n.notification_id),count(n.read_at)
 FROM hostelhive.notification_events e JOIN hostelhive.notification_outbox o ON o.event_id=e.event_id
 LEFT JOIN hostelhive.notifications n ON n.event_id=o.event_id AND n.recipient_id=o.recipient_id
 WHERE e.announcement_id=$1`, aid).Scan(&v.RecipientsTotal, &v.Delivered, &v.Read)
	if e != nil {
		return v, domain.ErrUnavailable
	}
	v.PendingDelivery = v.RecipientsTotal - v.Delivered
	v.Unread = v.Delivered - v.Read
	if tx.Commit(ctx) != nil {
		return v, domain.ErrUnavailable
	}
	return v, nil
}
