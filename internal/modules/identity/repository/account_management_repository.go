package repository

import (
	"context"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = "user_id::text,firebase_uid,email,role,is_active"

func scan(row pgx.Row) (domain.Account, error) {
	var a domain.Account
	err := row.Scan(&a.UserID, &a.FirebaseUID, &a.Email, &a.Role, &a.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, domain.ErrManagementNotFound
	}
	if err != nil {
		return a, domain.ErrManagementUnavailable
	}
	return a, nil
}

func (s *Store) List(ctx context.Context, limit, offset int) (domain.AccountPage, error) {
	page := domain.AccountPage{Users: []domain.Account{}, Limit: limit, Offset: offset}
	rows, err := s.pool.Query(ctx, "SELECT user_id::text,firebase_uid,email,role,is_active,full_name,designation FROM hostelhive.account_profiles JOIN hostelhive.users USING(user_id,firebase_uid,email,role,is_active) ORDER BY created_at,user_id LIMIT $1 OFFSET $2", limit+1, offset)
	if err != nil {
		return page, domain.ErrManagementUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var a domain.Account
		err := rows.Scan(&a.UserID, &a.FirebaseUID, &a.Email, &a.Role, &a.IsActive, &a.FullName, &a.Designation)
		if err != nil {
			return page, domain.ErrManagementUnavailable
		}
		page.Users = append(page.Users, a)
	}
	if rows.Err() != nil {
		return page, domain.ErrManagementUnavailable
	}
	if len(page.Users) > limit {
		page.HasMore = true
		page.Users = page.Users[:limit]
	}
	return page, nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// Serialize local account writes, then recheck both actor and last-admin state.
// This also coordinates with provisioning INSERTs and the first-admin bootstrap.

func (s *Store) Change(ctx context.Context, actor, id, role string, deactivate bool) (domain.AccountChange, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.AccountChange{}, domain.ErrManagementUnavailable
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return domain.AccountChange{}, domain.ErrManagementUnavailable
	}
	admin, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.users WHERE firebase_uid=$1", actor))
	if err != nil || !admin.IsActive || admin.Role != domain.RoleAdmin {
		if errors.Is(err, domain.ErrManagementUnavailable) {
			return domain.AccountChange{}, err
		}
		return domain.AccountChange{}, domain.ErrManagementForbidden
	}
	a, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.users WHERE user_id=$1", id))
	if err != nil {
		return domain.AccountChange{}, err
	}
	if a.IsActive && a.Role == domain.RoleAdmin && (deactivate || role != domain.RoleAdmin) {
		var n int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM hostelhive.users WHERE role='admin' AND is_active").Scan(&n); err != nil {
			return domain.AccountChange{}, domain.ErrManagementUnavailable
		}
		if n <= 1 {
			return domain.AccountChange{}, domain.ErrLastAdmin
		}
	}
	if deactivate {
		a, err = scan(tx.QueryRow(ctx, "UPDATE hostelhive.users SET is_active=false WHERE user_id=$1 RETURNING "+columns, id))
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO hostelhive.user_revocations(user_id) VALUES($1)
   ON CONFLICT(user_id) DO UPDATE SET requested_at=clock_timestamp(),next_attempt_at=clock_timestamp(),completed_at=NULL`, id)
		}
	} else {
		a, err = scan(tx.QueryRow(ctx, "UPDATE hostelhive.users SET role=$2 WHERE user_id=$1 RETURNING "+columns, id, role))
	}
	if err != nil {
		return domain.AccountChange{}, domain.ErrManagementUnavailable
	}
	if tx.Commit(ctx) != nil {
		return domain.AccountChange{}, domain.ErrManagementUnavailable
	}
	return domain.AccountChange{Account: a, RevocationPending: deactivate}, nil
}

func (s *Store) RetryOne(ctx context.Context, r service.Revoker, id string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, domain.ErrManagementUnavailable
	}
	defer rollback(tx)
	var userID, uid string
	err = tx.QueryRow(ctx, `SELECT j.user_id::text,u.firebase_uid FROM hostelhive.user_revocations j
 JOIN hostelhive.users u ON u.user_id=j.user_id WHERE j.completed_at IS NULL
 AND ($1::text='' OR j.user_id::text=$1) AND (j.next_attempt_at<=clock_timestamp() OR $1::text<>'')
 ORDER BY j.next_attempt_at,j.user_id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`, id).Scan(&userID, &uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, domain.ErrManagementUnavailable
	}
	revokeErr := r.DisableAndRevoke(ctx, uid)
	_, err = tx.Exec(ctx, `UPDATE hostelhive.user_revocations SET attempts=attempts+1,
 next_attempt_at=clock_timestamp()+interval '30 seconds',
 completed_at=CASE WHEN $2::boolean THEN clock_timestamp() ELSE NULL END WHERE user_id=$1`, userID, revokeErr == nil)
	if err != nil || tx.Commit(ctx) != nil {
		return false, domain.ErrManagementUnavailable
	}
	return true, revokeErr
}

func (s *Store) Pending(ctx context.Context, id string) (bool, error) {
	var pending bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.user_revocations WHERE user_id=$1 AND completed_at IS NULL)", id).Scan(&pending)
	if err != nil {
		return true, domain.ErrManagementUnavailable
	}
	return pending, nil
}

// RunRetries starts immediately and drains bounded batches. It exits before the
// server closes the database pool; pending rows survive process restarts.
