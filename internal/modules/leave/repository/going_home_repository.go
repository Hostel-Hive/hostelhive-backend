package repository

import (
	"context"
	"errors"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/jackc/pgx/v5"
)

const goingHomeColumns = "going_home_id::text,request_id::text,student_id::text,submitted_by::text,status,declared_at,departed_at,departure_device_id::text,departure_event_id::text"

func scanGoingHome(row pgx.Row) (domain.GoingHomeDeclaration, error) {
	var v domain.GoingHomeDeclaration
	if e := row.Scan(&v.GoingHomeID, &v.RequestID, &v.StudentID, &v.SubmittedBy, &v.Status, &v.DeclaredAt, &v.DepartedAt, &v.DepartureDeviceID, &v.DepartureEventID); e != nil {
		return v, dbError(e)
	}
	return v, nil
}

// The same Student lock as accepted scans and profile archival serializes intent,
// presence checks and departure. Authority cannot change until commit.
func (s *Store) DeclareGoingHome(ctx context.Context, uid string, in dto.GoingHomeInput) (domain.GoingHomeDeclaration, bool, error) {
	var empty domain.GoingHomeDeclaration
	tx, a, e := s.begin(ctx, uid)
	if e != nil {
		return empty, false, e
	}
	defer rollback(tx)
	if a.role != "student" {
		return empty, false, domain.ErrForbidden
	}
	eligible, e := lockStudent(ctx, tx, a.student)
	if e != nil {
		return empty, false, e
	}
	if !eligible {
		return empty, false, domain.ErrIneligible
	}
	var pendingSetup bool
	e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.student_invitations WHERE user_id=$1 AND setup_completed_at IS NULL)", a.id).Scan(&pendingSetup)
	if e != nil {
		return empty, false, dbError(e)
	}
	if pendingSetup {
		return empty, false, domain.ErrForbidden
	}
	old, e := scanGoingHome(tx.QueryRow(ctx, "SELECT "+goingHomeColumns+" FROM hostelhive.going_home_declarations WHERE student_id=$1 AND request_id=$2", a.student, in.RequestID))
	if e == nil {
		if e = tx.Commit(ctx); e != nil {
			return empty, false, dbError(e)
		}
		return old, true, nil
	}
	if !errors.Is(e, domain.ErrNotFound) {
		return empty, false, e
	}
	var presence string
	e = tx.QueryRow(ctx, "SELECT status FROM hostelhive.student_presence WHERE student_id=$1", a.student).Scan(&presence)
	if errors.Is(e, pgx.ErrNoRows) {
		return empty, false, domain.ErrPresenceUnknown
	}
	if e != nil {
		return empty, false, dbError(e)
	}
	if presence != "IN" {
		return empty, false, domain.ErrAlreadyOutside
	}
	var outstanding bool
	e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.going_home_declarations WHERE student_id=$1 AND status='declared')", a.student).Scan(&outstanding)
	if e != nil {
		return empty, false, dbError(e)
	}
	if outstanding {
		return empty, false, domain.ErrGoingHomePending
	}
	v, e := scanGoingHome(tx.QueryRow(ctx, "INSERT INTO hostelhive.going_home_declarations(request_id,student_id,submitted_by) VALUES($1,$2,$3) RETURNING "+goingHomeColumns, in.RequestID, a.student, a.id))
	if e != nil {
		return empty, false, e
	}
	if e = tx.Commit(ctx); e != nil {
		return empty, false, dbError(e)
	}
	return v, false, nil
}
