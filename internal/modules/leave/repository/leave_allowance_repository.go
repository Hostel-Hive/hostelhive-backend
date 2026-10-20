package repository

import (
	"context"
	"errors"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) AllowanceUsage(ctx context.Context, uid string, period domain.AllowancePeriod) (domain.AllowanceUsage, error) {
	var usage domain.AllowanceUsage
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return usage, err
	}
	defer rollback(tx)
	if a.role != "student" {
		return usage, domain.ErrForbidden
	}
	// The account lock and a shared Student lock keep authority and eligibility
	// stable through the read, using the same lock order as request decisions.
	var active, pendingSetup bool
	err = tx.QueryRow(ctx, `SELECT s.deleted_at IS NULL,
 EXISTS(SELECT 1 FROM hostelhive.student_invitations i WHERE i.user_id=s.user_id AND i.setup_completed_at IS NULL)
 FROM hostelhive.students s WHERE s.student_id=$1 FOR SHARE OF s`, a.student).Scan(&active, &pendingSetup)
	if errors.Is(err, pgx.ErrNoRows) {
		return usage, domain.ErrIneligible
	}
	if err != nil {
		return usage, dbError(err)
	}
	if !active {
		return usage, domain.ErrIneligible
	}
	if pendingSetup {
		return usage, domain.ErrForbidden
	}
	// Clip instants to this year BEFORE converting to local dates. PostgreSQL
	// stores microseconds, so end-1us is the last included instant in [start,end).
	// Union date ranges before counting: overlaps and adjacent ranges cannot
	// double-charge a date, and a long request never generates one row per day.
	const query = `WITH date_spans AS (
 SELECT daterange(
  (GREATEST(starts_at,$2::timestamptz) AT TIME ZONE $4)::date,
  ((LEAST(ends_at,$3::timestamptz)-interval '1 microsecond') AT TIME ZONE $4)::date+1,
  '[)') AS days
 FROM hostelhive.leave_requests
 WHERE student_id=$1 AND status='approved' AND starts_at<$3 AND ends_at>$2
 ), merged AS (SELECT range_agg(days) AS days FROM date_spans)
 SELECT COALESCE(sum(upper(span)-lower(span)),0)::int
 FROM merged CROSS JOIN LATERAL unnest(merged.days) AS spans(span)`
	usage.StudentID = a.student
	if err = tx.QueryRow(ctx, query, a.student, period.Start, period.End, period.Timezone).Scan(&usage.UsedDays); err != nil {
		return domain.AllowanceUsage{}, dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.AllowanceUsage{}, dbError(err)
	}
	return usage, nil
}
