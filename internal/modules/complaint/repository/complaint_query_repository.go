package repository

import (
	"context"
	"encoding/json"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
)

func (s *Store) List(ctx context.Context, uid string, f domain.Filter) (domain.Page, error) {
	p := domain.Page{Items: []domain.Complaint{}, Limit: f.Limit, Offset: f.Offset}
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return p, err
	}
	defer rollback(tx)
	if a.student != "" {
		if f.StudentID != "" && f.StudentID != a.student {
			return p, domain.ErrForbidden
		}
		f.StudentID = a.student
	}
	if a.role == "sub_warden" {
		if f.Category != "" && f.Category != "maintenance" {
			return p, domain.ErrForbidden
		}
		f.Category = "maintenance"
	}
	var raw []byte
	err = tx.QueryRow(ctx, `WITH matching AS (SELECT c.* FROM hostelhive.complaints c WHERE ($1::uuid IS NULL OR student_id=$1) AND ($2::text IS NULL OR category=$2) AND ($3::text IS NULL OR status=$3) AND ($4::timestamptz IS NULL OR created_at >= $4) AND ($5::timestamptz IS NULL OR created_at < $5)), page AS (SELECT * FROM matching ORDER BY created_at DESC,complaint_id DESC LIMIT $6 OFFSET $7)
 SELECT (SELECT count(*) FROM matching),COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.created_at DESC,p.complaint_id DESC) FROM page p),'[]'::jsonb)`, optional(f.StudentID), optional(f.Category), optional(f.Status), f.From, f.To, f.Limit, f.Offset).Scan(&p.Total, &raw)
	if err != nil {
		return p, dbError(err)
	}
	if json.Unmarshal(raw, &p.Items) != nil {
		return p, domain.ErrUnavailable
	}
	p.HasMore = int64(p.Offset)+int64(len(p.Items)) < p.Total
	if err = tx.Commit(ctx); err != nil {
		return p, dbError(err)
	}
	return p, nil
}
func (s *Store) Updates(ctx context.Context, uid, id string, limit, offset int) (domain.UpdatePage, error) {
	p := domain.UpdatePage{Items: []domain.Update{}, Limit: limit, Offset: offset}
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return p, err
	}
	defer rollback(tx)
	if _, err = get(ctx, tx, a, id, false); err != nil {
		return p, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, `WITH matching AS (SELECT * FROM hostelhive.complaint_updates WHERE complaint_id=$1), page AS (SELECT * FROM matching ORDER BY version LIMIT $2 OFFSET $3)
 SELECT (SELECT count(*) FROM matching),COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.version) FROM page p),'[]'::jsonb)`, id, limit, offset).Scan(&p.Total, &raw)
	if err != nil {
		return p, dbError(err)
	}
	if json.Unmarshal(raw, &p.Items) != nil {
		return p, domain.ErrUnavailable
	}
	p.HasMore = int64(offset)+int64(len(p.Items)) < p.Total
	if err = tx.Commit(ctx); err != nil {
		return p, dbError(err)
	}
	return p, nil
}
