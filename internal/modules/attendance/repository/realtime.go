package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	"github.com/jackc/pgx/v5"
)

func (s *Devices) realtimeTx(ctx context.Context, actor string) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
		rollback(tx)
		return nil, domain.ErrUnavailable
	}
	var allowed bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden'))", actor).Scan(&allowed); err != nil {
		rollback(tx)
		return nil, domain.ErrUnavailable
	}
	if !allowed {
		rollback(tx)
		return nil, domain.ErrForbidden
	}
	return tx, nil
}

// Intent is durable before external publication. A failed request returns 503;
// the worker retries, but clients may subscribe only after a successful session.
func (s *Devices) GrantSession(ctx context.Context, uid string, p service.RealtimePublisher) (domain.RealtimeSession, error) {
	tx, err := s.realtimeTx(ctx, uid)
	if err != nil {
		return domain.RealtimeSession{}, err
	}
	defer rollback(tx)
	var session domain.RealtimeSession
	err = tx.QueryRow(ctx, `INSERT INTO hostelhive.realtime_staff_grants(firebase_uid,version,expires_at)
 VALUES($1,(extract(epoch FROM clock_timestamp())*1000)::bigint,clock_timestamp()+interval '5 minutes')
 ON CONFLICT(firebase_uid) DO UPDATE SET version=greatest(hostelhive.realtime_staff_grants.version+1,EXCLUDED.version),
 expires_at=EXCLUDED.expires_at,pending=true,next_attempt_at=clock_timestamp()
 RETURNING expires_at`, uid).Scan(&session.ExpiresAt)
	if err != nil || tx.Commit(ctx) != nil {
		return session, domain.ErrUnavailable
	}
	worked, err := s.SyncGrant(ctx, p, uid)
	if err != nil || !worked {
		return session, domain.ErrUnavailable
	}
	// A concurrent deactivation between intent and sync must not report success.
	check, err := s.realtimeTx(ctx, uid)
	if err != nil {
		return session, err
	}
	defer rollback(check)
	var valid bool
	err = check.QueryRow(ctx, "SELECT NOT pending AND expires_at>clock_timestamp() FROM hostelhive.realtime_staff_grants WHERE firebase_uid=$1", uid).Scan(&valid)
	if err != nil || !valid || check.Commit(ctx) != nil {
		return session, domain.ErrUnavailable
	}
	return session, nil
}

// Grant sync holds current authorization against concurrent account mutations.
// Revocation tombstones keep versions, so delayed older writes cannot resurrect access.
func (s *Devices) SyncGrant(ctx context.Context, p service.RealtimePublisher, uid string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, domain.ErrUnavailable
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
		return false, domain.ErrUnavailable
	}
	var key string
	var g domain.Grant
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT firebase_uid,version,expires_at FROM hostelhive.realtime_staff_grants
 WHERE ($1='' OR firebase_uid=$1) AND
 ((pending AND (next_attempt_at<=clock_timestamp() OR $1<>'')) OR (expires_at>to_timestamp(0) AND expires_at<=clock_timestamp()))
 ORDER BY expires_at,firebase_uid LIMIT 1 FOR UPDATE SKIP LOCKED`, uid).Scan(&key, &g.Version, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, domain.ErrUnavailable
	}
	var allowed bool
	if tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden'))", key).Scan(&allowed) != nil {
		return false, domain.ErrUnavailable
	}
	if !allowed || !expires.After(time.Now()) {
		expires = time.Unix(0, 0)
		if tx.QueryRow(ctx, `UPDATE hostelhive.realtime_staff_grants SET expires_at=to_timestamp(0),
   version=greatest(version+1,(extract(epoch FROM clock_timestamp())*1000)::bigint),pending=true
   WHERE firebase_uid=$1 RETURNING version`, key).Scan(&g.Version) != nil {
			return false, domain.ErrUnavailable
		}
	}
	g.ExpiresAt = expires.UnixMilli()
	publishErr := p.SetGrant(ctx, key, g)
	_, err = tx.Exec(ctx, `UPDATE hostelhive.realtime_staff_grants SET pending=$2,attempts=least(attempts+1,1000000),
 next_attempt_at=clock_timestamp()+interval '1 second'*least(30,power(2,least(attempts,5))) WHERE firebase_uid=$1`, key, publishErr != nil)
	if err != nil || tx.Commit(ctx) != nil {
		return true, domain.ErrUnavailable
	}
	if publishErr != nil {
		return true, domain.ErrUnavailable
	}
	return true, nil
}

// Claim a short lease and a consistent snapshot, then release database locks
// before contacting Firebase. Acknowledgement cannot clear a newer revision.
func (s *Devices) PublishOne(ctx context.Context, p service.RealtimePublisher) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, domain.ErrUnavailable
	}
	defer rollback(tx)
	var projection domain.Projection
	var changed time.Time
	// Do not send fresh shared data while a known invalid grant still awaits
	// its cancellation write (including grants delayed by retry backoff).
	var blocked bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hostelhive.realtime_staff_grants g
 WHERE (g.pending OR g.expires_at>to_timestamp(0)) AND
 (g.expires_at<=clock_timestamp() OR NOT EXISTS(SELECT 1 FROM hostelhive.users u
 WHERE u.firebase_uid=g.firebase_uid AND u.is_active AND u.role IN ('admin','warden'))))`).Scan(&blocked)
	if err != nil {
		return false, domain.ErrUnavailable
	}
	if blocked {
		return false, nil
	}
	err = tx.QueryRow(ctx, `SELECT version,changed_at FROM hostelhive.attendance_publication
 WHERE pending AND next_attempt_at<=clock_timestamp() FOR UPDATE SKIP LOCKED`).Scan(&projection.Version, &changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, domain.ErrUnavailable
	}
	projection.ChangedAt = changed.UnixMilli()
	err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE p.status='IN'),
 count(*) FILTER(WHERE p.status='OUT'),count(*) FILTER(WHERE p.student_id IS NULL)
 FROM hostelhive.students s JOIN hostelhive.users u USING(user_id)
 LEFT JOIN hostelhive.student_presence p USING(student_id)
 WHERE s.deleted_at IS NULL AND u.is_active AND u.role='student'`).Scan(&projection.Summary.Total, &projection.Summary.In, &projection.Summary.Out, &projection.Summary.Unknown)
	if err != nil {
		return false, domain.ErrUnavailable
	}
	projection.PublishedAt = time.Now().UnixMilli()
	if _, err = tx.Exec(ctx, "UPDATE hostelhive.attendance_publication SET next_attempt_at=clock_timestamp()+interval '5 seconds'"); err != nil || tx.Commit(ctx) != nil {
		return false, domain.ErrUnavailable
	}
	publishErr := p.Publish(ctx, projection)
	_, err = s.pool.Exec(ctx, `UPDATE hostelhive.attendance_publication SET
 pending=CASE WHEN version=$2 AND NOT $1 THEN false ELSE pending END,
 attempts=CASE WHEN version<>$2 THEN attempts WHEN $1 THEN least(attempts+1,1000000) ELSE 0 END,
 next_attempt_at=CASE WHEN version=$2 THEN clock_timestamp()+interval '1 second'*least(30,power(2,least(attempts,5))) ELSE next_attempt_at END,
 published_at=CASE WHEN $1 THEN published_at ELSE clock_timestamp() END,
 published_version=CASE WHEN $1 THEN published_version ELSE greatest(coalesce(published_version,0),$2) END`, publishErr != nil, projection.Version)
	if err != nil {
		return true, domain.ErrUnavailable
	}
	if publishErr != nil {
		return true, domain.ErrUnavailable
	}
	return true, nil
}

func (s *Devices) Reconcile(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE hostelhive.attendance_publication SET
 version=greatest(version+1,(extract(epoch FROM clock_timestamp())*1000)::bigint),changed_at=clock_timestamp(),
 pending=true,next_attempt_at=clock_timestamp()
 WHERE NOT pending AND (published_at IS NULL OR published_at<=clock_timestamp()-interval '60 seconds')`)
	if err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Devices) PublicationStatus(ctx context.Context, actor string) (domain.PublicationStatus, error) {
	tx, err := s.realtimeTx(ctx, actor)
	if err != nil {
		return domain.PublicationStatus{}, err
	}
	defer rollback(tx)
	var status domain.PublicationStatus
	err = tx.QueryRow(ctx, `SELECT pending,version,published_version,changed_at,published_at,attempts,
 (SELECT count(*) FROM hostelhive.realtime_staff_grants WHERE pending)
 FROM hostelhive.attendance_publication`).Scan(&status.Pending, &status.Version, &status.PublishedVersion, &status.ChangedAt, &status.PublishedAt, &status.Attempts, &status.PendingGrants)
	if err != nil || tx.Commit(ctx) != nil {
		return status, domain.ErrUnavailable
	}
	return status, nil
}
