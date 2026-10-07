// Package accountmanagement manages current local roles and durable session revocation.
package accountmanagement

import (
	"context"
	"errors"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound    = errors.New("account not found")
	ErrForbidden   = errors.New("forbidden")
	ErrLastAdmin   = errors.New("last active administrator")
	ErrUnavailable = errors.New("account management unavailable")
)

type Page struct {
	Users   []authentication.Account `json:"users"`
	Limit   int                      `json:"limit"`
	Offset  int                      `json:"offset"`
	HasMore bool                     `json:"has_more"`
}
type Change struct {
	Account           authentication.Account `json:"account"`
	RevocationPending bool                   `json:"revocation_pending"`
}
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = "user_id::text,firebase_uid,email,role,is_active"

func scan(row pgx.Row) (authentication.Account, error) {
	var a authentication.Account
	err := row.Scan(&a.UserID, &a.FirebaseUID, &a.Email, &a.Role, &a.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, ErrUnavailable
	}
	return a, nil
}
func (s *Store) List(ctx context.Context, limit, offset int) (Page, error) {
	page := Page{Users: []authentication.Account{}, Limit: limit, Offset: offset}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" FROM hostelhive.users ORDER BY created_at,user_id LIMIT $1 OFFSET $2", limit+1, offset)
	if err != nil {
		return page, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return page, err
		}
		page.Users = append(page.Users, a)
	}
	if rows.Err() != nil {
		return page, ErrUnavailable
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
func (s *Store) Change(ctx context.Context, actor, id, role string, deactivate bool) (Change, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Change{}, ErrUnavailable
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return Change{}, ErrUnavailable
	}
	admin, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.users WHERE firebase_uid=$1", actor))
	if err != nil || !admin.IsActive || admin.Role != authentication.RoleAdmin {
		if errors.Is(err, ErrUnavailable) {
			return Change{}, err
		}
		return Change{}, ErrForbidden
	}
	a, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.users WHERE user_id=$1", id))
	if err != nil {
		return Change{}, err
	}
	if a.IsActive && a.Role == authentication.RoleAdmin && (deactivate || role != authentication.RoleAdmin) {
		var n int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM hostelhive.users WHERE role='admin' AND is_active").Scan(&n); err != nil {
			return Change{}, ErrUnavailable
		}
		if n <= 1 {
			return Change{}, ErrLastAdmin
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
		return Change{}, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Change{}, ErrUnavailable
	}
	return Change{Account: a, RevocationPending: deactivate}, nil
}

type Revoker interface {
	DisableAndRevoke(context.Context, string) error
}

// RetryOne holds a job row lock across external acknowledgements. Failed work
// stays durable and due after 30 seconds; a cancelled transaction stays pending.
func (s *Store) RetryOne(ctx context.Context, r Revoker, id string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, ErrUnavailable
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
		return false, ErrUnavailable
	}
	revokeErr := r.DisableAndRevoke(ctx, uid)
	_, err = tx.Exec(ctx, `UPDATE hostelhive.user_revocations SET attempts=attempts+1,
 next_attempt_at=clock_timestamp()+interval '30 seconds',
 completed_at=CASE WHEN $2::boolean THEN clock_timestamp() ELSE NULL END WHERE user_id=$1`, userID, revokeErr == nil)
	if err != nil || tx.Commit(ctx) != nil {
		return false, ErrUnavailable
	}
	return true, revokeErr
}
func (s *Store) Pending(ctx context.Context, id string) (bool, error) {
	var pending bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.user_revocations WHERE user_id=$1 AND completed_at IS NULL)", id).Scan(&pending)
	if err != nil {
		return true, ErrUnavailable
	}
	return pending, nil
}

// RunRetries starts immediately and drains bounded batches. It exits before the
// server closes the database pool; pending rows survive process restarts.
func (s *Store) RunRetries(ctx context.Context, r Revoker, timeout, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		for n := 0; n < 10 && ctx.Err() == nil; n++ {
			attempt, cancel := context.WithTimeout(ctx, timeout)
			worked, err := s.RetryOne(attempt, r, "")
			cancel()
			if !worked || err != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
