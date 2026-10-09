package identity_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
)

func TestPostgresAccountLookupIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a migrated test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal("connect test database failed")
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	uid := "issue15-" + time.Now().UTC().Format("20060102150405.000000000")
	_, err = tx.Exec(ctx, `INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES($1,$2,'student',true)`, uid, uid+"@example.invalid")
	if err != nil {
		t.Fatal("insert test fixture; ensure migrations through version 2 are applied")
	}
	store := repository.NewPostgresAccounts(tx)
	account, err := store.FindByFirebaseUID(ctx, uid)
	if err != nil || account.FirebaseUID != uid || account.Role != "student" || !account.IsActive {
		t.Fatalf("lookup: %+v %v", account, err)
	}
	_, err = tx.Exec(ctx, `UPDATE hostelhive.users SET role='warden',is_active=false WHERE firebase_uid=$1`, uid)
	if err != nil {
		t.Fatal(err)
	}
	account, err = store.FindByFirebaseUID(ctx, uid)
	if err != nil || account.Role != "warden" || account.IsActive {
		t.Fatal("current account state not loaded")
	}
	if _, err = store.FindByFirebaseUID(ctx, uid+"-missing"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Fatal("missing account not classified")
	}
}
