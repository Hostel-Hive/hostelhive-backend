package authentication

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}
type PostgresAccounts struct{ db Querier }

func NewPostgresAccounts(db Querier) *PostgresAccounts { return &PostgresAccounts{db: db} }

func (s *PostgresAccounts) FindByFirebaseUID(ctx context.Context, uid string) (Account, error) {
	var account Account
	err := s.db.QueryRow(ctx, `SELECT user_id::text, firebase_uid, email, role, is_active
 FROM hostelhive.users WHERE firebase_uid = $1`, uid).Scan(
		&account.UserID, &account.FirebaseUID, &account.Email, &account.Role, &account.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, errors.New("local account lookup failed")
	}
	return account, nil
}
