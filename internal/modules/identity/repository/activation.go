package repository

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	"github.com/jackc/pgx/v5"
)

type activationWork struct {
	tx      pgx.Tx
	account domain.Account
}

func (w *activationWork) Account() domain.Account { return w.account }
func (w *activationWork) Close()                  { rollback(w.tx) }
func (w *activationWork) Complete(ctx context.Context) (domain.Account, error) {
	a, err := scan(w.tx.QueryRow(ctx, "UPDATE hostelhive.users SET is_active=true WHERE user_id=$1 RETURNING "+columns, w.account.UserID))
	if err != nil || w.tx.Commit(ctx) != nil {
		return domain.Account{}, domain.ErrManagementUnavailable
	}
	return a, nil
}

// Hold account-write and revocation-row locks until Firebase acknowledges activation.
// A retry worker cannot disable this account after it becomes locally active.
func (s *Store) BeginActivation(ctx context.Context, actor, id string) (service.ActivationWork, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, domain.ErrManagementUnavailable
	}
	w := &activationWork{tx: tx}
	keep := false
	defer func() {
		if !keep {
			w.Close()
		}
	}()
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return nil, domain.ErrManagementUnavailable
	}
	admin, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.users WHERE firebase_uid=$1", actor))
	if errors.Is(err, domain.ErrManagementUnavailable) {
		return nil, err
	}
	if err != nil || !admin.IsActive || admin.Role != domain.RoleAdmin {
		return nil, domain.ErrManagementForbidden
	}
	w.account, err = scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.users WHERE user_id=$1", id))
	if err != nil {
		return nil, err
	}
	var completed bool
	err = tx.QueryRow(ctx, "SELECT completed_at IS NOT NULL FROM hostelhive.user_revocations WHERE user_id=$1 FOR UPDATE", id).Scan(&completed)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrManagementUnavailable
	}
	if err == nil && !completed {
		return nil, domain.ErrRevocationPending
	}
	var incomplete bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.user_provisioning WHERE firebase_uid=$1 AND completed_at IS NULL)", w.account.FirebaseUID).Scan(&incomplete); err != nil {
		return nil, domain.ErrManagementUnavailable
	}
	if incomplete {
		return nil, domain.ErrProvisioningIncomplete
	}
	keep = true
	return w, nil
}
