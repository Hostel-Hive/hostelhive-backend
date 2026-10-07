package authentication

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeRow struct {
	err     error
	account Account
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*string)) = r.account.UserID
	*(dest[1].(*string)) = r.account.FirebaseUID
	*(dest[2].(*string)) = r.account.Email
	*(dest[3].(*string)) = r.account.Role
	*(dest[4].(*bool)) = r.account.IsActive
	return nil
}

type queryFunc func(context.Context, string, ...any) pgx.Row

func (f queryFunc) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return f(ctx, sql, args...)
}
func TestAccountLookup(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want error
	}{{nil, nil}, {pgx.ErrNoRows, ErrAccountNotFound}, {errors.New("secret credentials"), errors.New("lookup failed")}} {
		store := NewPostgresAccounts(queryFunc(func(ctx context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "WHERE firebase_uid = $1") || len(args) != 1 || args[0] != "uid' OR TRUE" {
				t.Fatal("UID must be a bound parameter")
			}
			return fakeRow{err: tc.err, account: Account{UserID: "local", FirebaseUID: "uid", Role: "student", IsActive: true}}
		}))
		account, err := store.FindByFirebaseUID(context.Background(), "uid' OR TRUE")
		if tc.want == nil {
			if err != nil || account.UserID != "local" {
				t.Fatal(account, err)
			}
		} else if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal(err)
		}
		if tc.want == ErrAccountNotFound && !errors.Is(err, ErrAccountNotFound) {
			t.Fatal(err)
		}
	}
}
