package repository

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/jackc/pgx/v5"
)

// Called with the Student row locked. A failure rolls back scan, presence,
// consumption and incident intent together. Matching movements consume a grant
// even just before curfew; only unmatched restricted movements produce incidents.
func (s *Devices) evaluateAccess(ctx context.Context, tx pgx.Tx, e domain.Event) error {
	result := "not_evaluated"
	var approval *string
	if e.Outcome == "recorded" {
		lower, upper := domain.PermissionBounds(e.OccurredAt)
		var id string
		err := tx.QueryRow(ctx, `SELECT late_access_id::text FROM hostelhive.late_access_requests
 WHERE student_id=$1 AND direction=$2 AND status='approved' AND consumed_at IS NULL
 AND decided_at<=$3 AND expected_at BETWEEN $4 AND $5
 ORDER BY abs(extract(epoch FROM expected_at-$3::timestamptz)),expected_at,created_at,late_access_id
 LIMIT 1 FOR UPDATE`, e.StudentID, e.Direction, e.OccurredAt, lower, upper).Scan(&id)
		if err == nil {
			result = "approved"
			approval = &id
			if _, err = tx.Exec(ctx, `UPDATE hostelhive.late_access_requests SET consumed_at=$2,consumed_device_id=$3,consumed_event_id=$4 WHERE late_access_id=$1`, id, e.OccurredAt, e.DeviceID, e.EventID); err != nil {
				return domain.ErrUnavailable
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			result = "not_required"
			if s.accessPolicy.RequiresPermission(e.Direction, e.OccurredAt) {
				result = "unapproved"
				kind := "late_entry"
				if e.Direction == "OUT" {
					kind = "early_exit"
				}
				if _, err = tx.Exec(ctx, `INSERT INTO hostelhive.attendance_incidents(device_id,event_id,student_id,direction,incident_type,occurred_at,hostel_timezone,exit_opening_minute,entry_deadline_minute,permission_window_seconds)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, e.DeviceID, e.EventID, e.StudentID, e.Direction, kind, e.OccurredAt, s.accessPolicy.Location.String(), s.accessPolicy.ExitOpening, s.accessPolicy.EntryDeadline, int(domain.PermissionWindow.Seconds())); err != nil {
					return domain.ErrUnavailable
				}
			}
		} else {
			return domain.ErrUnavailable
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE hostelhive.attendance_events SET access_result=$3,late_access_id=$4 WHERE device_id=$1 AND event_id=$2`, e.DeviceID, e.EventID, result, approval); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
