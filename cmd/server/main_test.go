package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for _, tc := range []struct {
		name, environment, address string
		want                       int
	}{
		{"invalid_configuration", "invalid", "127.0.0.1:8080", 1},
		{"occupied_port", "test", listener.Addr().String(), 1},
		{"clean_shutdown", "test", "127.0.0.1:8080", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			address := tc.address
			if tc.name == "clean_shutdown" {
				free, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				address = free.Addr().String()
				free.Close()
			}
			t.Setenv("DATABASE_URL", "postgres://test:example@127.0.0.1:5432/hostelhive?sslmode=disable")
			t.Setenv("DATABASE_CHECK_TIMEOUT", "1s")
			t.Setenv("APP_ENV", tc.environment)
			t.Setenv("HTTP_ADDR", address)
			for _, key := range []string{"HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT"} {
				t.Setenv(key, "1s")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if code := run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))); code != tc.want {
				t.Fatalf("exit code = %d, want %d", code, tc.want)
			}
		})
	}
}
