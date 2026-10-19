package repository

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	"github.com/jackc/pgx/v5"
)

// Preparation and sending commit separately: the password baseline is durable
// before any email can be sent. No setup URL, action code or password is stored.
func (s *InvitationStore) DispatchInvitation(ctx context.Context, provider service.InvitationProvider) (bool, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return false, d.ErrInvitationUnavailable
	}
	defer rollback(tx)
	var id, ip string
	var baseline *float64
	var attempts int
	e = tx.QueryRow(ctx, `SELECT invitation_id::text,host(request_ip),baseline_password_updated_at,attempts FROM hostelhive.student_invitations
 WHERE delivery_status='pending' AND next_attempt_at<=clock_timestamp() ORDER BY next_attempt_at,invitation_id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &ip, &baseline, &attempts)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, d.ErrInvitationUnavailable
	}
	var uid, email, approved, role string
	var active, profile bool
	var done *time.Time
	e = tx.QueryRow(ctx, `SELECT u.firebase_uid,u.email,i.email,u.role,u.is_active,s.deleted_at IS NULL,i.setup_completed_at
 FROM hostelhive.student_invitations i JOIN hostelhive.users u ON u.user_id=i.user_id
 JOIN hostelhive.students s ON s.student_id=i.student_id AND s.user_id=u.user_id
 WHERE i.invitation_id=$1 FOR SHARE OF u,s`, id).Scan(&uid, &email, &approved, &role, &active, &profile, &done)
	if e != nil {
		return false, d.ErrInvitationUnavailable
	}
	if !active || !profile || role != "student" || email != approved || done != nil {
		if _, e = tx.Exec(ctx, "UPDATE hostelhive.student_invitations SET delivery_status='skipped',last_error='ineligible',next_attempt_at=NULL WHERE invitation_id=$1", id); e != nil {
			return false, d.ErrInvitationUnavailable
		}
		if e = invitationAudit(ctx, tx, id, nil, "skipped"); e != nil {
			return false, e
		}
		if tx.Commit(ctx) != nil {
			return false, d.ErrInvitationUnavailable
		}
		return true, nil
	}
	if attempts >= 8 {
		return false, d.ErrInvitationUnavailable
	}
	attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
	var providerError error
	if baseline == nil {
		var value float64
		value, providerError = provider.Prepare(attempt, uid, email)
		if providerError == nil && (math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value >= 1e16) {
			providerError = d.ErrInvitationUnavailable
		}
		if providerError == nil {
			_, e = tx.Exec(ctx, "UPDATE hostelhive.student_invitations SET baseline_password_updated_at=$2,attempts=attempts+1,last_error='',next_attempt_at=clock_timestamp() WHERE invitation_id=$1", id, value)
		}
	} else {
		providerError = provider.Send(attempt, uid, email, ip)
		if providerError == nil {
			_, e = tx.Exec(ctx, "UPDATE hostelhive.student_invitations SET delivery_status='sent',attempts=attempts+1,last_error='',last_sent_at=clock_timestamp(),next_attempt_at=NULL WHERE invitation_id=$1", id)
		}
	}
	cancel()
	if e != nil {
		return false, d.ErrInvitationUnavailable
	}
	action := "sent"
	if baseline == nil {
		action = "prepared"
	}
	if providerError != nil {
		state, code := "pending", "provider_unavailable"
		if attempts+1 >= 8 {
			state = "failed"
		}
		if errors.Is(providerError, d.ErrInvitationIdentity) {
			state, code = "failed", "identity_conflict"
		}
		delay := time.Duration(min(3600, 60*(1<<min(attempts, 6))))*time.Second + time.Duration(rand.IntN(15))*time.Second
		var retry *d.InvitationRetry
		if errors.As(providerError, &retry) && retry.After > delay {
			delay = retry.After
		}
		var next any
		if state == "pending" {
			next = time.Now().Add(delay)
		}
		if _, e = tx.Exec(ctx, "UPDATE hostelhive.student_invitations SET delivery_status=$2,attempts=attempts+1,last_error=$3,next_attempt_at=$4 WHERE invitation_id=$1", id, state, code, next); e != nil {
			return false, d.ErrInvitationUnavailable
		}
		action = "failed"
	}
	if e = invitationAudit(ctx, tx, id, nil, action); e != nil {
		return false, e
	}
	if tx.Commit(ctx) != nil {
		return false, d.ErrInvitationUnavailable
	}
	return true, nil
}
