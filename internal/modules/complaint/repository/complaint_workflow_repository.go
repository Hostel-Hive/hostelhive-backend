package repository

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/dto"
)

func (s *Store) Submit(ctx context.Context, uid string, in dto.Submit) (domain.Complaint, error) {
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return domain.Complaint{}, err
	}
	defer rollback(tx)
	if a.role != "student" {
		return domain.Complaint{}, domain.ErrForbidden
	}
	if err = eligible(ctx, tx, a); err != nil {
		return domain.Complaint{}, err
	}
	v, err := complaint(tx.QueryRow(ctx, `SELECT to_jsonb(c) FROM hostelhive.complaints c WHERE student_id=$1 AND request_id=$2`, a.student, in.RequestID))
	if err == nil {
		if v.Category != in.Category || v.Description != in.Description {
			return domain.Complaint{}, domain.ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.Complaint{}, dbError(err)
		}
		return v, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Complaint{}, err
	}
	v, err = complaint(tx.QueryRow(ctx, `INSERT INTO hostelhive.complaints(request_id,student_id,submitted_by,category,description) VALUES($1,$2,$3,$4,$5) RETURNING to_jsonb(complaints)`, in.RequestID, a.student, a.id, in.Category, in.Description))
	if err != nil {
		return domain.Complaint{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,new_status,text,previous_version,version,occurred_at) VALUES($1,$2,$3,'created','pending',$4,0,1,$5)`, v.ComplaintID, in.RequestID, a.id, in.Description, v.CreatedAt)
	if err != nil {
		return domain.Complaint{}, dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Complaint{}, dbError(err)
	}
	return v, nil
}
func (s *Store) Change(ctx context.Context, uid, id, action string, in dto.Change) (domain.Update, error) {
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return domain.Update{}, err
	}
	defer rollback(tx)
	if action == "status" && a.role == "student" {
		return domain.Update{}, domain.ErrForbidden
	}
	if a.role == "student" {
		if err = eligible(ctx, tx, a); err != nil {
			return domain.Update{}, err
		}
	}
	v, err := get(ctx, tx, a, id, true)
	if err != nil {
		return domain.Update{}, err
	}
	prior, err := update(tx.QueryRow(ctx, `SELECT to_jsonb(u) FROM hostelhive.complaint_updates u WHERE complaint_id=$1 AND actor_id=$2 AND request_id=$3`, id, a.id, in.RequestID))
	if err == nil {
		same := prior.Action == action && prior.Text == in.Text && prior.PreviousVersion == in.ExpectedVersion
		if action == "status" {
			same = same && prior.NewStatus == in.Status
		}
		if !same {
			return domain.Update{}, domain.ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.Update{}, dbError(err)
		}
		return prior, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Update{}, err
	}
	if v.Version != in.ExpectedVersion {
		return domain.Update{}, domain.ErrConflict
	}
	next := v.Status
	if action == "note" {
		if v.Status == "resolved" {
			return domain.Update{}, domain.ErrConflict
		}
	} else {
		switch v.Status {
		case "pending":
			if in.Status != "in_progress" && in.Status != "resolved" {
				return domain.Update{}, domain.ErrConflict
			}
		case "in_progress":
			if in.Status != "resolved" {
				return domain.Update{}, domain.ErrConflict
			}
		case "resolved":
			if in.Status != "in_progress" {
				return domain.Update{}, domain.ErrConflict
			}
			if in.Text == "" {
				return domain.Update{}, domain.ErrInvalid
			}
		}
		next = in.Status
	}
	_, err = tx.Exec(ctx, `UPDATE hostelhive.complaints SET status=$2,version=version+1,updated_at=GREATEST(clock_timestamp(),updated_at) WHERE complaint_id=$1`, id, next)
	if err != nil {
		return domain.Update{}, dbError(err)
	}
	result, err := update(tx.QueryRow(ctx, `INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,previous_status,new_status,text,previous_version,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8::bigint,$8::bigint+1) RETURNING to_jsonb(complaint_updates)`, id, in.RequestID, a.id, action, v.Status, next, in.Text, v.Version))
	if err != nil {
		return domain.Update{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Update{}, dbError(err)
	}
	return result, nil
}
