package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	"github.com/jackc/pgx/v5"
)

// OwnEvents derives ownership within a read-only snapshot shared by count and page.
func (s *Devices) OwnEvents(ctx context.Context, actor string, f dto.StudentHistoryFilter) (domain.StudentHistoryPage, error) {
	page := domain.StudentHistoryPage{Items: []domain.StudentMovement{}, Limit: f.Limit, Offset: f.Offset, Timezone: f.Timezone}
	if f.Start != nil {
		from, to := f.From, f.To
		page.From, page.To = &from, &to
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, domain.ErrUnavailable
	}
	defer rollback(tx)
	var allowed bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role='student')", actor).Scan(&allowed); err != nil {
		return page, domain.ErrUnavailable
	}
	if !allowed {
		return page, domain.ErrForbidden
	}
	var student string
	err = tx.QueryRow(ctx, "SELECT s.student_id::text FROM hostelhive.students s JOIN hostelhive.users u USING(user_id) WHERE u.firebase_uid=$1 AND s.deleted_at IS NULL", actor).Scan(&student)
	if errors.Is(err, pgx.ErrNoRows) {
		return page, domain.ErrNotFound
	}
	if err != nil {
		return page, domain.ErrUnavailable
	}
	const scope = ` FROM hostelhive.attendance_events WHERE student_id=$1 AND outcome='recorded'
 AND ($2::timestamptz IS NULL OR occurred_at >= $2)
 AND ($3::timestamptz IS NULL OR occurred_at < $3)
 AND ($4::text='' OR direction=$4)`
	if err = tx.QueryRow(ctx, "SELECT count(*)"+scope, student, f.Start, f.End, f.Direction).Scan(&page.Total); err != nil {
		return page, domain.ErrUnavailable
	}
	rows, err := tx.Query(ctx, "SELECT device_id::text,event_id::text,direction,occurred_at"+scope+" ORDER BY occurred_at DESC,device_id DESC,event_id DESC LIMIT $5 OFFSET $6", student, f.Start, f.End, f.Direction, f.Limit, f.Offset)
	if err != nil {
		return page, domain.ErrUnavailable
	}
	for rows.Next() {
		var device string
		var item domain.StudentMovement
		if err = rows.Scan(&device, &item.EventID, &item.Direction, &item.OccurredAt); err != nil {
			rows.Close()
			return page, domain.ErrUnavailable
		}
		// event_id is only unique within a device. Hide that device ID while exposing
		// a stable list key derived from the complete immutable database identity.
		sum := sha256.Sum256([]byte(device + ":" + item.EventID))
		item.MovementID = hex.EncodeToString(sum[:])
		page.Items = append(page.Items, item)
	}
	rows.Close()
	if rows.Err() != nil {
		return page, domain.ErrUnavailable
	}
	page.HasMore = int64(f.Offset)+int64(len(page.Items)) < page.Total
	if tx.Commit(ctx) != nil {
		return domain.StudentHistoryPage{}, domain.ErrUnavailable
	}
	return page, nil
}
