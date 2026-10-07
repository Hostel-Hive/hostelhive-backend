package postgres_test

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/platform/postgres"
	"os"
	"testing"
	"time"
)

func TestPoolPostgreSQLIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	pool, err := postgres.Open(context.Background(), url, 2*time.Second)
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
