package repository

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Devices) SetDirection(ctx context.Context, actor, id, direction string) (domain.Device, error) {
	tx, actorID, err := s.begin(ctx, actor)
	if err != nil {
		return domain.Device{}, err
	}
	defer rollback(tx)
	d, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.scanner_devices WHERE device_id=$1 FOR UPDATE", id))
	if err != nil {
		return domain.Device{}, err
	}
	if d.Direction == nil || *d.Direction != direction {
		d, err = scan(tx.QueryRow(ctx, "UPDATE hostelhive.scanner_devices SET direction=$2,updated_at=clock_timestamp(),updated_by=$3 WHERE device_id=$1 RETURNING "+columns, id, direction, actorID))
		if err != nil {
			return domain.Device{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO hostelhive.scanner_device_audit(device_id,action,credential_version,actor_id,direction) VALUES($1,'direction_set',$2,$3,$4)", id, d.CredentialVersion, actorID, direction); err != nil {
			return domain.Device{}, domain.ErrUnavailable
		}
	}
	if tx.Commit(ctx) != nil {
		return domain.Device{}, domain.ErrUnavailable
	}
	return d, nil
}

const eventColumns = "event_id::text,device_id::text,student_id::text,direction,outcome,occurred_at"

func scanEvent(row pgx.Row) (domain.Event, error) {
	var e domain.Event
	err := row.Scan(&e.EventID, &e.DeviceID, &e.StudentID, &e.Direction, &e.Outcome, &e.OccurredAt)
	return e, err
}

// Record locks authority until commit. Device version fences authentication
// performed before a concurrent rotation; direction is read inside this tx.
// Student locking serializes events across all devices and profile archival.
func (s *Devices) Record(ctx context.Context, authenticated domain.Device, eventID, token string, hash []byte, cooldown time.Duration) (domain.Event, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Event{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
		return domain.Event{}, domain.ErrUnavailable
	}
	d, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.scanner_devices WHERE device_id=$1 FOR SHARE", authenticated.DeviceID))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Event{}, domain.ErrUnauthorized
		}
		return domain.Event{}, err
	}
	if !d.IsActive || d.CredentialVersion != authenticated.CredentialVersion {
		return domain.Event{}, domain.ErrUnauthorized
	}
	// Serialize reuse of an event ID even when requests resolve different students.
	// Hash collisions only cause extra serialization, never false identity matches.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", d.DeviceID+":"+eventID); err != nil {
		return domain.Event{}, domain.ErrUnavailable
	}
	var prior domain.Event
	var priorHash []byte
	err = tx.QueryRow(ctx, "SELECT "+eventColumns+",payload_hash FROM hostelhive.attendance_events WHERE device_id=$1 AND event_id=$2", d.DeviceID, eventID).Scan(&prior.EventID, &prior.DeviceID, &prior.StudentID, &prior.Direction, &prior.Outcome, &prior.OccurredAt, &priorHash)
	if err == nil {
		if subtle.ConstantTimeCompare(hash, priorHash) != 1 {
			return domain.Event{}, domain.ErrEventConflict
		}
		if tx.Commit(ctx) != nil {
			return domain.Event{}, domain.ErrUnavailable
		}
		return prior, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Event{}, domain.ErrUnavailable
	}
	if d.Direction == nil {
		return domain.Event{}, domain.ErrDirection
	}
	var studentID string
	err = tx.QueryRow(ctx, `SELECT s.student_id::text FROM hostelhive.student_qr q
 JOIN hostelhive.students s USING(student_id) JOIN hostelhive.users u USING(user_id)
 WHERE q.token=$1 AND s.deleted_at IS NULL AND u.is_active AND u.role='student'
 FOR UPDATE OF s`, token).Scan(&studentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Event{}, domain.ErrStudent
	}
	if err != nil {
		return domain.Event{}, domain.ErrUnavailable
	}
	var now time.Time
	if tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now) != nil {
		return domain.Event{}, domain.ErrUnavailable
	}
	var latest time.Time
	err = tx.QueryRow(ctx, "SELECT occurred_at FROM hostelhive.attendance_events WHERE student_id=$1 AND direction=$2 AND outcome='recorded' ORDER BY occurred_at DESC LIMIT 1", studentID, *d.Direction).Scan(&latest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.Event{}, domain.ErrUnavailable
	}
	outcome := "recorded"
	if err == nil && now.Sub(latest) < cooldown {
		outcome = "cooldown"
	} else {
		var status string
		err = tx.QueryRow(ctx, "SELECT status FROM hostelhive.student_presence WHERE student_id=$1", studentID).Scan(&status)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return domain.Event{}, domain.ErrUnavailable
		}
		if err == nil && status == *d.Direction {
			if status == "IN" {
				outcome = "already_in"
			} else {
				outcome = "already_out"
			}
		}
	}
	e, err := scanEvent(tx.QueryRow(ctx, "INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING "+eventColumns, eventID, d.DeviceID, studentID, hash, *d.Direction, outcome, now))
	if err != nil {
		return domain.Event{}, domain.ErrUnavailable
	}
	if outcome == "recorded" {
		_, err = tx.Exec(ctx, `INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(student_id) DO UPDATE SET status=EXCLUDED.status,updated_at=EXCLUDED.updated_at,device_id=EXCLUDED.device_id,event_id=EXCLUDED.event_id`, studentID, *d.Direction, now, d.DeviceID, eventID)
		if err != nil {
			return domain.Event{}, domain.ErrUnavailable
		}
	}
	if tx.Commit(ctx) != nil {
		return domain.Event{}, domain.ErrUnavailable
	}
	return e, nil
}

func (s *Devices) readAttendance(ctx context.Context, actor, id string) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
		rollback(tx)
		return nil, domain.ErrUnavailable
	}
	var allowed bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden'))", actor).Scan(&allowed)
	if err != nil {
		rollback(tx)
		return nil, domain.ErrUnavailable
	}
	if !allowed {
		rollback(tx)
		return nil, domain.ErrForbidden
	}
	// Historical attendance remains readable after profile archival/deactivation.
	var found bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.students WHERE student_id=$1)", id).Scan(&found)
	if err != nil {
		rollback(tx)
		return nil, domain.ErrUnavailable
	}
	if !found {
		rollback(tx)
		return nil, domain.ErrNotFound
	}
	return tx, nil
}

func (s *Devices) Presence(ctx context.Context, actor, id string) (domain.Presence, error) {
	tx, err := s.readAttendance(ctx, actor, id)
	if err != nil {
		return domain.Presence{}, err
	}
	defer rollback(tx)
	p := domain.Presence{StudentID: id, Status: "UNKNOWN"}
	err = tx.QueryRow(ctx, "SELECT status,updated_at FROM hostelhive.student_presence WHERE student_id=$1", id).Scan(&p.Status, &p.UpdatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.Presence{}, domain.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return domain.Presence{}, domain.ErrUnavailable
	}
	return p, nil
}

func (s *Devices) Events(ctx context.Context, actor, id string, limit, offset int) (domain.EventPage, error) {
	tx, err := s.readAttendance(ctx, actor, id)
	if err != nil {
		return domain.EventPage{}, err
	}
	defer rollback(tx)
	p := domain.EventPage{Items: []domain.Event{}, Limit: limit, Offset: offset}
	rows, err := tx.Query(ctx, "SELECT "+eventColumns+" FROM hostelhive.attendance_events WHERE student_id=$1 ORDER BY occurred_at DESC,device_id,event_id LIMIT $2 OFFSET $3", id, limit+1, offset)
	if err != nil {
		return domain.EventPage{}, domain.ErrUnavailable
	}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			rows.Close()
			return domain.EventPage{}, domain.ErrUnavailable
		}
		p.Items = append(p.Items, e)
	}
	rows.Close()
	if rows.Err() != nil {
		return domain.EventPage{}, domain.ErrUnavailable
	}
	if len(p.Items) > limit {
		p.HasMore = true
		p.Items = p.Items[:limit]
	}
	if tx.Commit(ctx) != nil {
		return domain.EventPage{}, domain.ErrUnavailable
	}
	return p, nil
}
