package repository

import (
	"context"
	"errors"

	pgx "github.com/jackc/pgx/v5"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
)

type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PostgresAccounts struct{ db Querier }

func NewPostgresAccounts(db Querier) *PostgresAccounts { return &PostgresAccounts{db: db} }

func (s *PostgresAccounts) FindByFirebaseUID(ctx context.Context, uid string) (domain.Account, error) {
	var account domain.Account
	err := s.db.QueryRow(ctx, `SELECT user_id::text, firebase_uid, email, role, is_active
 FROM hostelhive.users WHERE firebase_uid = $1`, uid).Scan(
		&account.UserID, &account.FirebaseUID, &account.Email, &account.Role, &account.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	if err != nil {
		return domain.Account{}, errors.New("local account lookup failed")
	}
	return account, nil
}
