package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/jackc/pgx/v5"
	"math/big"
	"time"
)

func retryDelay(attempt int, provider time.Duration) time.Duration {
	delay := time.Minute * time.Duration(1<<min(max(attempt-1, 0), 6))
	if delay > time.Hour {
		delay = time.Hour
	}
	jitter, _ := rand.Int(rand.Reader, big.NewInt(int64(delay/10)+1))
	if jitter != nil {
		delay += time.Duration(jitter.Int64())
	}
	if provider > delay {
		delay = provider
	}
	return delay
}

// One pending row is locked until the bounded provider attempt and acknowledgment
// finish. A crash rolls back; an ambiguous provider success may be retried using
// the SAME notification ID. Network sends are not exactly once.
func (s *Store) DispatchPush(ctx context.Context, send service.PushSend) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	defer rollback(tx)
	var id, nid, registration string
	var generation int64
	var attempts int
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT delivery_id::text,notification_id::text,registration_id::text,generation,attempts,expires_at
 FROM hostelhive.push_deliveries WHERE state='pending' AND (next_attempt_at<=clock_timestamp() OR expires_at<=clock_timestamp())
 ORDER BY next_attempt_at,delivery_id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &nid, &registration, &generation, &attempts, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	finish := func(state, code string) (int, error) {
		if _, err := tx.Exec(ctx, `UPDATE hostelhive.push_deliveries SET state=$2,last_code=$3 WHERE delivery_id=$1`, id, state, code); err != nil {
			return 0, domain.ErrUnavailable
		}
		if tx.Commit(ctx) != nil {
			return 0, domain.ErrUnavailable
		}
		return 1, nil
	}
	if !expires.After(time.Now()) {
		return finish("expired", "expired")
	}
	if attempts >= domain.PushMaxAttempts {
		return finish("failed", "attempt_limit")
	}
	var user, role string
	var active bool
	// Per-recipient row lock prevents a completed deactivation/role change from
	// being bypassed by an already-authorized send; no whole-users-table lock.
	err = tx.QueryRow(ctx, `SELECT u.user_id::text,u.role,u.is_active FROM hostelhive.users u JOIN hostelhive.notifications n ON n.recipient_id=u.user_id WHERE n.notification_id=$1 FOR SHARE OF u`, nid).Scan(&user, &role, &active)
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	if !active {
		return finish("skipped", "account_inactive")
	}
	var token string
	var current int64
	var registered bool
	err = tx.QueryRow(ctx, `SELECT token,generation,active FROM hostelhive.push_devices WHERE registration_id=$1 AND user_id=$2 FOR UPDATE`, registration, user).Scan(&token, &current, &registered)
	if errors.Is(err, pgx.ErrNoRows) {
		return finish("skipped", "device_unavailable")
	}
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	if !registered || current != generation {
		return finish("skipped", "device_changed")
	}
	var message domain.PushMessage
	err = tx.QueryRow(ctx, `SELECT n.notification_id::text,e.source_type,e.kind`+visible+` AND n.notification_id=$3`, user, role, nid).Scan(&message.NotificationID, &message.SourceType, &message.Kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return finish("skipped", "permission_changed")
	}
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	if message.Kind != "leave_decided" && message.Kind != "late_access_decided" && message.Kind != "attendance_incident" {
		return finish("skipped", "event_not_eligible")
	}
	message.Token, message.ExpiresAt = token, expires
	result := send(ctx, message)
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	attempts++
	state, code := "pending", "provider_unavailable"
	var providerID any
	var accepted any
	next := time.Now().Add(retryDelay(attempts, result.RetryAfter))
	switch result.State {
	case "accepted":
		if result.ProviderID != "" && len(result.ProviderID) <= 512 {
			state, code = "accepted", "accepted"
			providerID = result.ProviderID
			accepted = time.Now()
		}
	case "invalid_token":
		state, code = "failed", "unregistered"
		if _, err := tx.Exec(ctx, `UPDATE hostelhive.push_devices SET active=false,token='',generation=generation+1,updated_at=clock_timestamp() WHERE registration_id=$1`, registration); err != nil {
			return 0, domain.ErrUnavailable
		}
	case "failed":
		state, code = "failed", "provider_rejected"
	}
	if state == "pending" && attempts >= domain.PushMaxAttempts {
		state, code = "failed", "attempt_limit"
	}
	if _, err := tx.Exec(ctx, `UPDATE hostelhive.push_deliveries SET state=$2,attempts=$3,next_attempt_at=$4,provider_message_id=$5,accepted_at=$6,last_code=$7 WHERE delivery_id=$1`, id, state, attempts, next, providerID, accepted, code); err != nil {
		return 0, domain.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return 0, domain.ErrUnavailable
	}
	return 1, nil
}
