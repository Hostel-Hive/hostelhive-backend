package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Store) pushActor(ctx context.Context, uid string) (pgx.Tx, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", domain.ErrUnavailable
	}
	var id string
	// Serialize registrations/caps and stabilize current account authority.
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden','sub_warden','student','security_staff') FOR UPDATE`, uid).Scan(&id)
	if err != nil {
		rollback(tx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", domain.ErrForbidden
		}
		return nil, "", domain.ErrUnavailable
	}
	return tx, id, nil
}
func (s *Store) RegisterPushDevice(ctx context.Context, uid, id string, v dto.PushRegistration) (domain.PushDevice, error) {
	tx, user, err := s.pushActor(ctx, uid)
	if err != nil {
		return domain.PushDevice{}, err
	}
	defer rollback(tx)
	var active, count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE device_id=$2),count(*) FROM hostelhive.push_devices WHERE user_id=$1 AND active`, user, id).Scan(&active, &count); err != nil {
		return domain.PushDevice{}, domain.ErrUnavailable
	}
	if active == 0 && count >= domain.PushDeviceLimit {
		return domain.PushDevice{}, domain.ErrPushConflict
	}
	hash := sha256.Sum256([]byte(v.Token))
	var device domain.PushDevice
	err = tx.QueryRow(ctx, `INSERT INTO hostelhive.push_devices(user_id,device_id,platform,token,token_hash) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(user_id,device_id) DO UPDATE SET platform=EXCLUDED.platform,token=EXCLUDED.token,token_hash=EXCLUDED.token_hash,active=true,
 generation=CASE WHEN push_devices.active AND push_devices.token=EXCLUDED.token AND push_devices.platform=EXCLUDED.platform THEN push_devices.generation ELSE push_devices.generation+1 END,
 bound_at=CASE WHEN push_devices.active AND push_devices.token=EXCLUDED.token AND push_devices.platform=EXCLUDED.platform THEN push_devices.bound_at ELSE clock_timestamp() END,
 updated_at=clock_timestamp()
 RETURNING device_id::text,platform,active,updated_at`, user, id, v.Platform, v.Token, hash[:]).Scan(&device.DeviceID, &device.Platform, &device.Active, &device.UpdatedAt)
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" {
			return device, domain.ErrPushConflict
		}
		return device, domain.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return domain.PushDevice{}, domain.ErrUnavailable
	}
	return device, nil
}
func (s *Store) ListPushDevices(ctx context.Context, uid string) (domain.PushDevicePage, error) {
	page := domain.PushDevicePage{Items: []domain.PushDevice{}}
	tx, user, err := s.pushActor(ctx, uid)
	if err != nil {
		return page, err
	}
	defer rollback(tx)
	rows, err := tx.Query(ctx, `SELECT device_id::text,platform,active,updated_at FROM hostelhive.push_devices WHERE user_id=$1 AND active ORDER BY updated_at DESC,device_id LIMIT 10`, user)
	if err != nil {
		return page, domain.ErrUnavailable
	}
	for rows.Next() {
		var d domain.PushDevice
		if rows.Scan(&d.DeviceID, &d.Platform, &d.Active, &d.UpdatedAt) != nil {
			rows.Close()
			return page, domain.ErrUnavailable
		}
		page.Items = append(page.Items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || tx.Commit(ctx) != nil {
		return page, domain.ErrUnavailable
	}
	page.Total = len(page.Items)
	return page, nil
}
func (s *Store) RemovePushDevice(ctx context.Context, uid, id string) error {
	tx, user, err := s.pushActor(ctx, uid)
	if err != nil {
		return err
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, `UPDATE hostelhive.push_devices SET generation=CASE WHEN active THEN generation+1 ELSE generation END,active=false,token='',updated_at=clock_timestamp() WHERE user_id=$1 AND device_id=$2`, user, id)
	if err != nil {
		return domain.ErrUnavailable
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if tx.Commit(ctx) != nil {
		return domain.ErrUnavailable
	}
	return nil
}
