package repository

import (
	"context"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	"github.com/jackc/pgx/v5"
)

// Lock current account authority before taking the query snapshot. The page and
// its total use one repeatable-read snapshot, even as scanners update presence.
func (s *Devices) dashboardTx(ctx context.Context, actor string) (pgx.Tx, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
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
	return tx, nil
}

// The same eligibility and search scope backs the list and all three counts.
const dashboardFrom = ` FROM hostelhive.students s
 JOIN hostelhive.users u USING(user_id)
 LEFT JOIN hostelhive.student_presence p USING(student_id)
 WHERE s.deleted_at IS NULL AND u.is_active AND u.role='student'
 AND ($1='' OR s.full_name ILIKE $1 ESCAPE '!' OR s.index_no ILIKE $1 ESCAPE '!')`

func searchPattern(query string) string {
	if query == "" {
		return ""
	}
	return "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(query) + "%"
}

func (s *Devices) PresenceStudents(ctx context.Context, actor string, filter dto.PresenceFilter) (domain.PresencePage, error) {
	tx, err := s.dashboardTx(ctx, actor)
	if err != nil {
		return domain.PresencePage{}, err
	}
	defer rollback(tx)
	page := domain.PresencePage{Items: []domain.PresenceStudent{}, Limit: filter.Limit, Offset: filter.Offset}
	scope := dashboardFrom + " AND ($2='' OR COALESCE(p.status,'UNKNOWN')=$2)"
	pattern := searchPattern(filter.Query)
	if err = tx.QueryRow(ctx, "SELECT count(*)"+scope, pattern, filter.Status).Scan(&page.Total); err != nil {
		return domain.PresencePage{}, domain.ErrUnavailable
	}
	rows, err := tx.Query(ctx, "SELECT s.student_id::text,s.index_no,s.full_name,COALESCE(p.status,'UNKNOWN'),p.updated_at"+scope+" ORDER BY lower(s.full_name),lower(s.index_no),s.student_id LIMIT $3 OFFSET $4", pattern, filter.Status, filter.Limit, filter.Offset)
	if err != nil {
		return domain.PresencePage{}, domain.ErrUnavailable
	}
	for rows.Next() {
		var item domain.PresenceStudent
		if err = rows.Scan(&item.StudentID, &item.IndexNo, &item.FullName, &item.Status, &item.UpdatedAt); err != nil {
			rows.Close()
			return domain.PresencePage{}, domain.ErrUnavailable
		}
		page.Items = append(page.Items, item)
	}
	rows.Close()
	if rows.Err() != nil {
		return domain.PresencePage{}, domain.ErrUnavailable
	}
	page.HasMore = int64(filter.Offset)+int64(len(page.Items)) < page.Total
	if tx.Commit(ctx) != nil {
		return domain.PresencePage{}, domain.ErrUnavailable
	}
	return page, nil
}

func (s *Devices) PresenceSummary(ctx context.Context, actor, query string) (domain.PresenceSummary, error) {
	tx, err := s.dashboardTx(ctx, actor)
	if err != nil {
		return domain.PresenceSummary{}, err
	}
	defer rollback(tx)
	var summary domain.PresenceSummary
	err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE p.status='IN'),count(*) FILTER(WHERE p.status='OUT'),count(*) FILTER(WHERE p.student_id IS NULL)`+dashboardFrom, searchPattern(query)).Scan(&summary.Total, &summary.In, &summary.Out, &summary.Unknown)
	if err != nil {
		return domain.PresenceSummary{}, domain.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return domain.PresenceSummary{}, domain.ErrUnavailable
	}
	return summary, nil
}
