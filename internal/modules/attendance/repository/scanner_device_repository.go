package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	accessdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Devices struct {
	pool         *pgxpool.Pool
	accessPolicy accessdomain.Policy
}

func NewDevices(pool *pgxpool.Pool) *Devices {
	return NewDevicesWithPolicy(pool, accessdomain.Policy{Location: time.FixedZone("Asia/Colombo", 19800), ExitOpening: 360, EntryDeadline: 1200})
}
func NewDevicesWithPolicy(pool *pgxpool.Pool, policy accessdomain.Policy) *Devices {
	return &Devices{pool: pool, accessPolicy: policy}
}

const columns = "device_id::text,name,is_active,credential_version,created_at,updated_at,created_by::text,updated_by::text,direction"

func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" {
			return domain.ErrConflict
		}
		if p.Code == "23514" || p.Code == "22P02" || p.Code == "22001" {
			return domain.ErrInvalid
		}
	}
	return domain.ErrUnavailable
}

func scan(row pgx.Row) (domain.Device, error) {
	var d domain.Device
	err := row.Scan(&d.DeviceID, &d.Name, &d.IsActive, &d.CredentialVersion, &d.CreatedAt, &d.UpdatedAt, &d.CreatedBy, &d.UpdatedBy, &d.Direction)
	if err != nil {
		return domain.Device{}, dbError(err)
	}
	return d, nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func (s *Devices) begin(ctx context.Context, actor string) (pgx.Tx, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", domain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
		rollback(tx)
		return nil, "", domain.ErrUnavailable
	}
	var actorID string
	err = tx.QueryRow(ctx, "SELECT user_id::text FROM hostelhive.users WHERE firebase_uid=$1 AND role='admin' AND is_active", actor).Scan(&actorID)
	if err != nil {
		rollback(tx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", domain.ErrForbidden
		}
		return nil, "", domain.ErrUnavailable
	}
	return tx, actorID, nil
}

func audit(ctx context.Context, tx pgx.Tx, d domain.Device, actorID, action string) error {
	_, err := tx.Exec(ctx, "INSERT INTO hostelhive.scanner_device_audit(device_id,action,credential_version,actor_id) VALUES($1,$2,$3,$4)", d.DeviceID, action, d.CredentialVersion, actorID)
	return err
}

func (s *Devices) Register(ctx context.Context, actor, id, name string, hash []byte) (domain.Device, error) {
	tx, actorID, err := s.begin(ctx, actor)
	if err != nil {
		return domain.Device{}, err
	}
	defer rollback(tx)
	d, err := scan(tx.QueryRow(ctx, "INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,created_by,updated_by) VALUES($1,$2,$3,$4,$4) RETURNING "+columns, id, name, hash, actorID))
	if err != nil {
		return domain.Device{}, err
	}
	if err = audit(ctx, tx, d, actorID, "registered"); err != nil {
		return domain.Device{}, dbError(err)
	}
	if tx.Commit(ctx) != nil {
		return domain.Device{}, domain.ErrUnavailable
	}
	return d, nil
}

func (s *Devices) List(ctx context.Context, actor string, limit, offset int) (domain.DevicePage, error) {
	tx, _, err := s.begin(ctx, actor)
	if err != nil {
		return domain.DevicePage{}, err
	}
	defer rollback(tx)
	page := domain.DevicePage{Items: []domain.Device{}, Limit: limit, Offset: offset}
	rows, err := tx.Query(ctx, "SELECT "+columns+" FROM hostelhive.scanner_devices ORDER BY created_at,device_id LIMIT $1 OFFSET $2", limit+1, offset)
	if err != nil {
		return domain.DevicePage{}, dbError(err)
	}
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			rows.Close()
			return domain.DevicePage{}, err
		}
		page.Items = append(page.Items, d)
	}
	rows.Close()
	if rows.Err() != nil {
		return domain.DevicePage{}, domain.ErrUnavailable
	}
	if len(page.Items) > limit {
		page.HasMore = true
		page.Items = page.Items[:limit]
	}
	if tx.Commit(ctx) != nil {
		return domain.DevicePage{}, domain.ErrUnavailable
	}
	return page, nil
}

func (s *Devices) Rotate(ctx context.Context, actor, id string, hash []byte) (domain.Device, error) {
	tx, actorID, err := s.begin(ctx, actor)
	if err != nil {
		return domain.Device{}, err
	}
	defer rollback(tx)
	d, err := scan(tx.QueryRow(ctx, "UPDATE hostelhive.scanner_devices SET credential_hash=$2,credential_version=credential_version+1,updated_at=clock_timestamp(),updated_by=$3 WHERE device_id=$1 RETURNING "+columns, id, hash, actorID))
	if err != nil {
		return domain.Device{}, err
	}
	if err = audit(ctx, tx, d, actorID, "rotated"); err != nil {
		return domain.Device{}, dbError(err)
	}
	if tx.Commit(ctx) != nil {
		return domain.Device{}, domain.ErrUnavailable
	}
	return d, nil
}

func (s *Devices) Deactivate(ctx context.Context, actor, id string) (domain.Device, error) {
	tx, actorID, err := s.begin(ctx, actor)
	if err != nil {
		return domain.Device{}, err
	}
	defer rollback(tx)
	d, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.scanner_devices WHERE device_id=$1 FOR UPDATE", id))
	if err != nil {
		return domain.Device{}, err
	}
	if d.IsActive {
		d, err = scan(tx.QueryRow(ctx, "UPDATE hostelhive.scanner_devices SET is_active=false,updated_at=clock_timestamp(),updated_by=$2 WHERE device_id=$1 RETURNING "+columns, id, actorID))
		if err != nil {
			return domain.Device{}, err
		}
		if err = audit(ctx, tx, d, actorID, "deactivated"); err != nil {
			return domain.Device{}, dbError(err)
		}
	}
	if tx.Commit(ctx) != nil {
		return domain.Device{}, domain.ErrUnavailable
	}
	return d, nil
}

// No credential cache: every authentication observes current committed state.
func (s *Devices) Authentication(ctx context.Context, id string) (domain.AuthenticationMaterial, error) {
	var a domain.AuthenticationMaterial
	err := s.pool.QueryRow(ctx, "SELECT "+columns+",credential_hash FROM hostelhive.scanner_devices WHERE device_id=$1", id).Scan(&a.Device.DeviceID, &a.Device.Name, &a.Device.IsActive, &a.Device.CredentialVersion, &a.Device.CreatedAt, &a.Device.UpdatedAt, &a.Device.CreatedBy, &a.Device.UpdatedBy, &a.Device.Direction, &a.Hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AuthenticationMaterial{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.AuthenticationMaterial{}, domain.ErrUnavailable
	}
	return a, nil
}
