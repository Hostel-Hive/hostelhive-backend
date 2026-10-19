package repository

import (
	"bytes"
	"context"
	"errors"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InvitationStore struct{ pool *pgxpool.Pool }

func NewInvitationStore(pool *pgxpool.Pool) *InvitationStore { return &InvitationStore{pool} }

const invitationColumns = "i.invitation_id::text,i.request_id::text,i.user_id::text,i.student_id::text,i.email,i.delivery_status,i.attempts,i.last_error,i.created_at,i.last_requested_at,i.next_attempt_at,i.last_sent_at,i.setup_completed_at"

func invitationError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return d.ErrInvitationNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		return d.ErrInvitationConflict
	}
	return d.ErrInvitationUnavailable
}
func scanInvitation(row pgx.Row) (d.StudentInvitation, error) {
	var v d.StudentInvitation
	e := row.Scan(&v.InvitationID, &v.RequestID, &v.UserID, &v.StudentID, &v.Email, &v.DeliveryStatus, &v.Attempts, &v.LastError, &v.CreatedAt, &v.LastRequestedAt, &v.NextAttemptAt, &v.LastSentAt, &v.SetupCompletedAt)
	if e != nil {
		return v, invitationError(e)
	}
	return v, nil
}
func getInvitation(ctx context.Context, tx pgx.Tx, id string) (d.StudentInvitation, error) {
	return scanInvitation(tx.QueryRow(ctx, "SELECT "+invitationColumns+" FROM hostelhive.student_invitations i WHERE i.invitation_id=$1", id))
}
func (s *InvitationStore) admin(ctx context.Context, actor string, read bool) (pgx.Tx, string, error) {
	opts := pgx.TxOptions{}
	if read {
		opts = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	}
	tx, e := s.pool.BeginTx(ctx, opts)
	if e != nil {
		return nil, "", d.ErrInvitationUnavailable
	}
	if !read {
		if _, e = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"); e != nil {
			rollback(tx)
			return nil, "", d.ErrInvitationUnavailable
		}
	}
	var id string
	e = tx.QueryRow(ctx, "SELECT user_id::text FROM hostelhive.users WHERE firebase_uid=$1 AND role='admin' AND is_active AND NOT EXISTS(SELECT 1 FROM hostelhive.student_invitations i WHERE i.user_id=users.user_id AND i.setup_completed_at IS NULL)", actor).Scan(&id)
	if e != nil {
		rollback(tx)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, "", d.ErrInvitationForbidden
		}
		return nil, "", d.ErrInvitationUnavailable
	}
	return tx, id, nil
}
func invitationAudit(ctx context.Context, tx pgx.Tx, id string, actor *string, action string) error {
	if _, e := tx.Exec(ctx, "INSERT INTO hostelhive.student_invitation_audit(invitation_id,actor_id,action) VALUES($1,$2,$3)", id, actor, action); e != nil {
		return d.ErrInvitationUnavailable
	}
	return nil
}
func (s *InvitationStore) RegisterInvitation(ctx context.Context, actor, ip string, v dto.StudentInvitationInput, hash []byte) (d.StudentInvitation, bool, error) {
	tx, admin, e := s.admin(ctx, actor, false)
	if e != nil {
		return d.StudentInvitation{}, false, e
	}
	defer rollback(tx)
	var oldID string
	var oldHash []byte
	e = tx.QueryRow(ctx, "SELECT invitation_id::text,payload_hash FROM hostelhive.student_invitations WHERE created_by=$1 AND request_id=$2", admin, v.RequestID).Scan(&oldID, &oldHash)
	if e == nil {
		if !bytes.Equal(hash, oldHash) {
			return d.StudentInvitation{}, false, d.ErrInvitationConflict
		}
		old, e := getInvitation(ctx, tx, oldID)
		if e != nil {
			return old, false, e
		}
		if tx.Commit(ctx) != nil {
			return old, false, d.ErrInvitationUnavailable
		}
		return old, true, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return d.StudentInvitation{}, false, d.ErrInvitationUnavailable
	}
	var reserved bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.user_provisioning WHERE email=$1)", v.Email).Scan(&reserved); e != nil {
		return d.StudentInvitation{}, false, d.ErrInvitationUnavailable
	}
	if reserved {
		return d.StudentInvitation{}, false, d.ErrInvitationConflict
	}
	var user, student, id string
	e = tx.QueryRow(ctx, "INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,'student',true) RETURNING user_id::text", "hhi_"+uuid.NewString(), v.Email).Scan(&user)
	if e != nil {
		return d.StudentInvitation{}, false, invitationError(e)
	}
	e = tx.QueryRow(ctx, "INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,$4,$5,$6) RETURNING student_id::text", user, v.IndexNo, v.FullName, v.Faculty, v.Year, v.ContactPhone).Scan(&student)
	if e != nil {
		return d.StudentInvitation{}, false, invitationError(e)
	}
	for _, g := range v.Guardians {
		if _, e = tx.Exec(ctx, "INSERT INTO hostelhive.guardians(student_id,name,relationship,contact_phone) VALUES($1,$2,$3,$4)", student, g.Name, g.Relationship, g.ContactPhone); e != nil {
			return d.StudentInvitation{}, false, invitationError(e)
		}
	}
	e = tx.QueryRow(ctx, "INSERT INTO hostelhive.student_invitations(request_id,user_id,student_id,email,created_by,request_ip,payload_hash) VALUES($1,$2,$3,$4,$5,$6::inet,$7) RETURNING invitation_id::text", v.RequestID, user, student, v.Email, admin, ip, hash).Scan(&id)
	if e != nil {
		return d.StudentInvitation{}, false, invitationError(e)
	}
	if e = invitationAudit(ctx, tx, id, &admin, "requested"); e != nil {
		return d.StudentInvitation{}, false, e
	}
	result, e := getInvitation(ctx, tx, id)
	if e != nil {
		return result, false, e
	}
	if tx.Commit(ctx) != nil {
		return d.StudentInvitation{}, false, d.ErrInvitationUnavailable
	}
	return result, false, nil
}
func (s *InvitationStore) GetInvitation(ctx context.Context, actor, id string) (d.StudentInvitation, error) {
	tx, _, e := s.admin(ctx, actor, true)
	if e != nil {
		return d.StudentInvitation{}, e
	}
	defer rollback(tx)
	v, e := getInvitation(ctx, tx, id)
	if e != nil {
		return v, e
	}
	if tx.Commit(ctx) != nil {
		return v, d.ErrInvitationUnavailable
	}
	return v, nil
}
func (s *InvitationStore) ListInvitations(ctx context.Context, actor string, limit, offset int) (d.InvitationPage, error) {
	page := d.InvitationPage{Items: []d.StudentInvitation{}, Limit: limit, Offset: offset}
	tx, _, e := s.admin(ctx, actor, true)
	if e != nil {
		return page, e
	}
	defer rollback(tx)
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM hostelhive.student_invitations").Scan(&page.Total); e != nil {
		return page, d.ErrInvitationUnavailable
	}
	rows, e := tx.Query(ctx, "SELECT "+invitationColumns+" FROM hostelhive.student_invitations i ORDER BY i.created_at DESC,i.invitation_id DESC LIMIT $1 OFFSET $2", limit, offset)
	if e != nil {
		return page, d.ErrInvitationUnavailable
	}
	for rows.Next() {
		v, e := scanInvitation(rows)
		if e != nil {
			rows.Close()
			return page, e
		}
		page.Items = append(page.Items, v)
	}
	rows.Close()
	if rows.Err() != nil {
		return page, d.ErrInvitationUnavailable
	}
	page.HasMore = int64(offset)+int64(len(page.Items)) < page.Total
	if tx.Commit(ctx) != nil {
		return page, d.ErrInvitationUnavailable
	}
	return page, nil
}
func (s *InvitationStore) ResendInvitation(ctx context.Context, actor, id, request, ip string) (d.StudentInvitation, error) {
	tx, admin, e := s.admin(ctx, actor, false)
	if e != nil {
		return d.StudentInvitation{}, e
	}
	defer rollback(tx)
	v, e := getInvitation(ctx, tx, id)
	if e != nil {
		return v, e
	}
	var previous string
	e = tx.QueryRow(ctx, "SELECT actor_id::text FROM hostelhive.student_invitation_resends WHERE invitation_id=$1 AND request_id=$2", id, request).Scan(&previous)
	if e == nil {
		if previous != admin {
			return v, d.ErrInvitationConflict
		}
		if tx.Commit(ctx) != nil {
			return v, d.ErrInvitationUnavailable
		}
		return v, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return v, d.ErrInvitationUnavailable
	}
	var eligible bool
	e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users u JOIN hostelhive.students s USING(user_id) WHERE u.user_id=$1 AND u.is_active AND u.role='student' AND u.email=$2 AND s.student_id=$3 AND s.deleted_at IS NULL)", v.UserID, v.Email, v.StudentID).Scan(&eligible)
	if e != nil {
		return v, d.ErrInvitationUnavailable
	}
	if !eligible {
		return v, d.ErrInvitationConflict
	}
	// Serialize with workers/completion as well as other Admin requests.
	var completed *time.Time
	var status string
	var last time.Time
	e = tx.QueryRow(ctx, "SELECT setup_completed_at,delivery_status,last_requested_at FROM hostelhive.student_invitations WHERE invitation_id=$1 FOR UPDATE", id).Scan(&completed, &status, &last)
	if e != nil {
		return v, d.ErrInvitationUnavailable
	}
	if completed != nil {
		return v, d.ErrInvitationConflict
	}
	var now time.Time
	if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
		return v, d.ErrInvitationUnavailable
	}
	if status != "pending" && now.Sub(last) < 5*time.Minute {
		return v, d.ErrInvitationRateLimited
	}
	if _, e = tx.Exec(ctx, "INSERT INTO hostelhive.student_invitation_resends(invitation_id,request_id,actor_id) VALUES($1,$2,$3)", id, request, admin); e != nil {
		return v, invitationError(e)
	}
	if status != "pending" {
		if _, e = tx.Exec(ctx, "UPDATE hostelhive.student_invitations SET delivery_status='pending',attempts=0,last_error='',next_attempt_at=clock_timestamp(),last_requested_at=clock_timestamp(),request_ip=$2::inet WHERE invitation_id=$1", id, ip); e != nil {
			return v, d.ErrInvitationUnavailable
		}
	}
	if e = invitationAudit(ctx, tx, id, &admin, "resend_requested"); e != nil {
		return v, e
	}
	v, e = getInvitation(ctx, tx, id)
	if e != nil {
		return v, e
	}
	if tx.Commit(ctx) != nil {
		return v, d.ErrInvitationUnavailable
	}
	return v, nil
}
func (s *InvitationStore) CompleteSetup(ctx context.Context, actor string, verify func(context.Context, string, string, float64) error) (d.SetupResult, error) {
	var result d.SetupResult
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return result, d.ErrInvitationUnavailable
	}
	defer rollback(tx)
	if _, e = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); e != nil {
		return result, d.ErrInvitationUnavailable
	}
	var user, uid, email string
	e = tx.QueryRow(ctx, "SELECT user_id::text,firebase_uid,email FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role='student'", actor).Scan(&user, &uid, &email)
	if errors.Is(e, pgx.ErrNoRows) {
		return result, d.ErrInvitationForbidden
	}
	if e != nil {
		return result, d.ErrInvitationUnavailable
	}
	var profile bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.students WHERE user_id=$1 AND deleted_at IS NULL)", user).Scan(&profile); e != nil {
		return result, d.ErrInvitationUnavailable
	}
	if !profile {
		return result, d.ErrInvitationNotFound
	}
	var id, approved string
	var baseline *float64
	e = tx.QueryRow(ctx, "SELECT invitation_id::text,email,baseline_password_updated_at,setup_completed_at FROM hostelhive.student_invitations WHERE user_id=$1 FOR UPDATE", user).Scan(&id, &approved, &baseline, &result.SetupCompletedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return result, nil
	}
	if e != nil {
		return result, d.ErrInvitationUnavailable
	}
	if email != approved {
		return result, d.ErrInvitationConflict
	}
	if result.SetupCompletedAt != nil {
		return result, nil
	}
	if baseline == nil {
		return result, d.ErrSetupPending
	}
	if e = verify(ctx, uid, email, *baseline); e != nil {
		return result, e
	}
	e = tx.QueryRow(ctx, "UPDATE hostelhive.student_invitations SET setup_completed_at=clock_timestamp(),delivery_status=CASE WHEN delivery_status='pending' THEN 'skipped' ELSE delivery_status END,next_attempt_at=NULL WHERE invitation_id=$1 RETURNING setup_completed_at", id).Scan(&result.SetupCompletedAt)
	if e != nil {
		return result, d.ErrInvitationUnavailable
	}
	if e = invitationAudit(ctx, tx, id, &user, "completed"); e != nil {
		return result, e
	}
	if tx.Commit(ctx) != nil {
		return result, d.ErrInvitationUnavailable
	}
	return result, nil
}
