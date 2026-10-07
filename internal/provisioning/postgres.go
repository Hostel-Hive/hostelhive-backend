package provisioning

import (
	"context"
	"errors"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (s *PostgresRepository) Reserve(ctx context.Context, r Reservation) error {
	// A different key must not adopt an existing local account or reservation.
	_, err := s.pool.Exec(ctx, `INSERT INTO hostelhive.user_provisioning
 (request_key,requester_uid,firebase_uid,email,role)
 SELECT $1::text,$2::text,$3::text,$4::text,$5::text WHERE NOT EXISTS(SELECT 1 FROM hostelhive.users WHERE lower(email)=$4::text)
 ON CONFLICT(request_key) DO NOTHING`, r.Key, r.RequesterUID, r.FirebaseUID, r.Email, r.Role)
	if err != nil {
		return databaseError(err)
	}
	var existing Reservation
	err = s.pool.QueryRow(ctx, `SELECT requester_uid,email,role FROM hostelhive.user_provisioning WHERE request_key=$1`, r.Key).Scan(&existing.RequesterUID, &existing.Email, &existing.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return databaseError(err)
	}
	if existing.RequesterUID != r.RequesterUID || existing.Email != r.Email || existing.Role != r.Role {
		return ErrConflict
	}
	return nil
}

func (s *PostgresRepository) Begin(ctx context.Context, key string) (Work, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	work := &postgresWork{tx: tx}
	err = tx.QueryRow(ctx, `SELECT request_key,requester_uid,firebase_uid,email,role,completed_at IS NOT NULL
 FROM hostelhive.user_provisioning WHERE request_key=$1 FOR UPDATE NOWAIT`, key).Scan(
		&work.reservation.Key, &work.reservation.RequesterUID, &work.reservation.FirebaseUID, &work.reservation.Email, &work.reservation.Role, &work.reservation.Completed)
	if err != nil {
		work.Close()
		return nil, databaseError(err)
	}
	return work, nil
}

type postgresWork struct {
	tx          pgx.Tx
	reservation Reservation
}

func (w *postgresWork) Reservation() Reservation { return w.reservation }
func (w *postgresWork) Account(ctx context.Context) (authentication.Account, error) {
	var account authentication.Account
	err := w.tx.QueryRow(ctx, `SELECT u.user_id::text,u.firebase_uid,u.email,u.role,u.is_active
 FROM hostelhive.users u JOIN hostelhive.user_provisioning p ON p.user_id=u.user_id WHERE p.request_key=$1`, w.reservation.Key).Scan(
		&account.UserID, &account.FirebaseUID, &account.Email, &account.Role, &account.IsActive)
	if err != nil {
		return authentication.Account{}, databaseError(err)
	}
	return account, nil
}
func (w *postgresWork) InsertInactive(ctx context.Context) (authentication.Account, error) {
	var account authentication.Account
	r := w.reservation
	err := w.tx.QueryRow(ctx, `INSERT INTO hostelhive.users(firebase_uid,email,role,is_active)
 VALUES($1,$2,$3,false) RETURNING user_id::text,firebase_uid,email,role,is_active`, r.FirebaseUID, r.Email, r.Role).Scan(
		&account.UserID, &account.FirebaseUID, &account.Email, &account.Role, &account.IsActive)
	if err != nil {
		return authentication.Account{}, databaseError(err)
	}
	return account, nil
}
func (w *postgresWork) Complete(ctx context.Context, userID string) error {
	tag, err := w.tx.Exec(ctx, `UPDATE hostelhive.users SET is_active=true WHERE user_id=$1 AND firebase_uid=$2`, userID, w.reservation.FirebaseUID)
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrUnavailable
	}
	tag, err = w.tx.Exec(ctx, `UPDATE hostelhive.user_provisioning SET user_id=$2,completed_at=clock_timestamp() WHERE request_key=$1`, w.reservation.Key, userID)
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrUnavailable
	}
	if err := w.tx.Commit(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}
func (w *postgresWork) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = w.tx.Rollback(ctx)
}
func databaseError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrConflict
		}
		if pgErr.Code == "55P03" {
			return ErrInProgress
		}
	}
	return ErrUnavailable
}
