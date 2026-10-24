package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestCORSConfiguration(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  []string
	}{
		{"", nil},
		{"   ", nil},
		{"https://web-kohl-delta-90.vercel.app", []string{"https://web-kohl-delta-90.vercel.app"}},
		{" https://WEB-kohl-delta-90.vercel.app/ , http://localhost:5173,https://web-kohl-delta-90.vercel.app", []string{"https://web-kohl-delta-90.vercel.app", "http://localhost:5173"}},
		{"https://example.com:443,http://example.com:80,http://[::1]:5173", []string{"https://example.com", "http://example.com", "http://[::1]:5173"}},
	} {
		cfg, err := load(fromMap(map[string]string{"FIREBASE_PROJECT_ID": "test", "DATABASE_URL": "postgres://test:example@localhost/test", "CORS_ALLOWED_ORIGINS": tc.value}))
		if err != nil || !reflect.DeepEqual(cfg.CORSAllowedOrigins, tc.want) {
			t.Fatalf("origins: got %v, want %v, err %v", cfg.CORSAllowedOrigins, tc.want, err)
		}
	}
	for _, value := range []string{
		"*", "https://*.vercel.app", "null", "https://example.com,", "https://example.com,,http://localhost:5173",
		"example.com", "//example.com", "ftp://example.com", "https://", "https://user:private@example.com",
		"https://example.com/path", "https://example.com?x=1", "https://example.com?", "https://example.com#fragment", "https://example.com#",
		"https://example.com:0", "https://example.com:65536", "https://example.com:invalid", "https://example.com:", "https://[not-an-ip]",
		"https://example.com http://evil.example", "https://example.com\r\nX-Injected:value",
	} {
		cfg, err := load(fromMap(map[string]string{"FIREBASE_PROJECT_ID": "test", "DATABASE_URL": "postgres://test:example@localhost/test", "CORS_ALLOWED_ORIGINS": value}))
		if err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") || strings.Contains(err.Error(), "private") || len(cfg.CORSAllowedOrigins) != 0 {
			t.Fatalf("invalid origins accepted or echoed: err %v", err)
		}
	}
}

func TestCORSUnsetByDefault(t *testing.T) {
	cfg, err := load(fromMap(map[string]string{"FIREBASE_PROJECT_ID": "test", "DATABASE_URL": "postgres://test:example@localhost/test"}))
	if err != nil || len(cfg.CORSAllowedOrigins) != 0 {
		t.Fatal("CORS must be opt-in", err)
	}
}
