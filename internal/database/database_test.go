package database

import (
	"context"
	"os"
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

func TestPoolPostgreSQLIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	pool, err := Open(context.Background(), url, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal("PostgreSQL ping failed")
	}
	pool.Close()
	if pool.Stat().TotalConns() != 0 {
		t.Fatal("pool connections remain after Close")
	}
	if err := pool.Ping(ctx); err == nil {
		t.Fatal("closed pool must reject readiness checks")
	}
}
