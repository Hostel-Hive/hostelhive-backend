package postgres

import (
	"context"

	"strings"
	"testing"
	"time"
)

func TestOpenInvalidSettingsRedactsCredentials(t *testing.T) {
	for _, value := range []string{
		"postgres://user:secret@example/db?pool_max_conns=invalid",
		"postgres://user:secret@example/db?sslmode=invalid",
	} {
		_, err := Open(context.Background(), value, time.Second)
		if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "example") {
			t.Fatalf("expected redacted configuration error, got %v", err)
		}
	}
	if _, err := Open(context.Background(), "postgres://user:secret@example/db", 0); err == nil {
		t.Fatal("zero check timeout must fail")
	}
}

func TestPoolUnavailableDatabase(t *testing.T) {
	pool, err := Open(context.Background(), "postgres://test:example@127.0.0.1:1/test?sslmode=disable", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := pool.Ping(ctx); err == nil {
		t.Fatal("expected unavailable database to fail readiness")
	}
}
