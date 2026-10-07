package config

import (
	"strings"
	"testing"
	"time"
)

func fromMap(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { v, ok := values[key]; return v, ok }
}

func TestDefaults(t *testing.T) {
	cfg, err := load(fromMap(map[string]string{"DATABASE_URL": "postgres://test:example@localhost:5432/hostelhive?sslmode=disable"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "development" || cfg.HTTPAddr != "127.0.0.1:8080" ||
		cfg.ReadHeaderTimeout != 5*time.Second || cfg.ReadTimeout != 15*time.Second ||
		cfg.WriteTimeout != 15*time.Second || cfg.IdleTimeout != time.Minute ||
		cfg.ShutdownTimeout != 10*time.Second || cfg.DatabaseCheckTimeout != 2*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestOverrides(t *testing.T) {
	cfg, err := load(fromMap(map[string]string{
		"DATABASE_URL": "postgres://test:example@localhost:5432/hostelhive?sslmode=disable", "DATABASE_CHECK_TIMEOUT": "3s",
		"APP_ENV": "production", "HTTP_ADDR": " [::1]:9090 ",
		"HTTP_READ_HEADER_TIMEOUT": "2s", "HTTP_READ_TIMEOUT": "20s",
		"HTTP_WRITE_TIMEOUT": "30s", "HTTP_IDLE_TIMEOUT": "2m", "HTTP_SHUTDOWN_TIMEOUT": "3s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "production" || cfg.HTTPAddr != "[::1]:9090" ||
		cfg.ReadHeaderTimeout != 2*time.Second || cfg.ReadTimeout != 20*time.Second ||
		cfg.WriteTimeout != 30*time.Second || cfg.IdleTimeout != 2*time.Minute || cfg.ShutdownTimeout != 3*time.Second || cfg.DatabaseCheckTimeout != 3*time.Second {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	cases := []struct{ key, value string }{
		{"APP_ENV", ""}, {"APP_ENV", "staging"},
		{"HTTP_ADDR", ""}, {"HTTP_ADDR", "8080"}, {"HTTP_ADDR", "localhost:0"},
		{"HTTP_ADDR", "localhost:65536"}, {"HTTP_ADDR", "localhost:http"},
		{"HTTP_ADDR", "unknown-host:8080"},
	}
	for _, key := range []string{"HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT", "DATABASE_CHECK_TIMEOUT"} {
		for _, value := range []string{"", "nonsense", "0s", "-1s", "999999999999999999h"} {
			cases = append(cases, struct{ key, value string }{key, value})
		}
	}
	for _, tc := range cases {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			_, err := load(fromMap(map[string]string{"DATABASE_URL": "postgres://test:example@localhost:5432/hostelhive?sslmode=disable", tc.key: tc.value}))
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("expected error identifying %s, got %v", tc.key, err)
			}
		})
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:example@localhost:5432/hostelhive?sslmode=disable")
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDR", "localhost:9090")
	for _, key := range []string{"HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT", "DATABASE_CHECK_TIMEOUT"} {
		t.Setenv(key, "1s")
	}
	cfg, err := Load()
	if err != nil || cfg.Environment != "test" || cfg.HTTPAddr != "localhost:9090" {
		t.Fatalf("environment not loaded: %+v, %v", cfg, err)
	}
}

func TestDatabaseURLValidation(t *testing.T) {
	for _, value := range []string{"", " ", "bad", "https://user:secret@localhost/db", "postgres://localhost/db", "postgres://user:secret@localhost", "postgres://user:secret@%/db"} {
		_, err := load(fromMap(map[string]string{"DATABASE_URL": value}))
		if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") || strings.Contains(err.Error(), "secret") {
			t.Fatalf("expected safe DATABASE_URL validation error, got %v", err)
		}
	}
	if _, err := load(fromMap(nil)); err == nil {
		t.Fatal("missing DATABASE_URL must fail")
	}
}
