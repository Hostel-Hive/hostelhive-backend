package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
)

func requestLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log line is not JSON: %q", raw)
		}
		lines = append(lines, line)
	}
	buf.Reset()
	return lines
}

func TestRequestLogRecordsRefusedAndFailedRequests(t *testing.T) {
	verifier := verifyFunc(func(_ context.Context, raw string) (string, error) {
		switch raw {
		case "student-secret-token":
			return "uid-student", nil
		case "admin-secret-token":
			return "uid-admin", nil
		case "outage-secret-token":
			return "uid-outage", nil
		}
		return "", errors.New("invalid token")
	})
	accounts := accountFunc(func(_ context.Context, uid string) (domain.Account, error) {
		switch uid {
		case "uid-student":
			return domain.Account{UserID: "11111111-1111-1111-1111-111111111111", FirebaseUID: uid, Email: "student@stu.kln.ac.lk", Role: domain.RoleStudent, IsActive: true}, nil
		case "uid-admin":
			return domain.Account{UserID: "22222222-2222-2222-2222-222222222222", FirebaseUID: uid, Email: "admin@kln.ac.lk", Role: domain.RoleAdmin, IsActive: true}, nil
		}
		return domain.Account{}, errors.New("database password authentication failed")
	})
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/users/{userID}", Middleware(verifier, accounts, time.Second)(RequireRoles(domain.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	var buf bytes.Buffer
	handler := RequestLog(slog.New(slog.NewJSONHandler(&buf, nil)))(mux)

	for _, tc := range []struct {
		name, path, token string
		status            int
		level, route      string
		actor             string
	}{
		{"missing token", "/api/v1/users/33333333-3333-3333-3333-333333333333?email=student@stu.kln.ac.lk", "", 401, "WARN", "/api/v1/users/{userID}", ""},
		{"forged token", "/api/v1/users/33333333-3333-3333-3333-333333333333", "forged-secret-token", 401, "WARN", "/api/v1/users/{userID}", ""},
		{"wrong role", "/api/v1/users/33333333-3333-3333-3333-333333333333", "student-secret-token", 403, "WARN", "/api/v1/users/{userID}", "11111111-1111-1111-1111-111111111111"},
		{"allowed", "/api/v1/users/33333333-3333-3333-3333-333333333333", "admin-secret-token", 200, "INFO", "/api/v1/users/{userID}", "22222222-2222-2222-2222-222222222222"},
		{"database outage", "/api/v1/users/33333333-3333-3333-3333-333333333333", "outage-secret-token", 503, "ERROR", "/api/v1/users/{userID}", ""},
		{"unknown route", "/api/v1/missing", "", 404, "INFO", "unmatched", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			raw := buf.String()
			lines := requestLogLines(t, &buf)
			if rec.Code != tc.status || len(lines) != 1 {
				t.Fatalf("status %d, %d log lines", rec.Code, len(lines))
			}
			line := lines[0]
			if line["level"] != tc.level || line["route"] != tc.route || line["method"] != "GET" || line["status"] != float64(tc.status) {
				t.Fatalf("unexpected log line %v", line)
			}
			if _, ok := line["duration_ms"].(float64); !ok {
				t.Fatalf("duration missing: %v", line)
			}
			if got, _ := line["actor_id"].(string); got != tc.actor {
				t.Fatalf("actor_id = %q, want %q", got, tc.actor)
			}
			if tc.actor != "" && line["actor_type"] != "user" {
				t.Fatalf("actor_type = %v", line["actor_type"])
			}
			for _, leak := range []string{"secret-token", "Bearer", "@", "33333333", "?", "password", "uid-"} {
				if strings.Contains(raw, leak) {
					t.Fatalf("log line leaks %q: %s", leak, raw)
				}
			}
		})
	}

	t.Run("healthy probes are not logged", func(t *testing.T) {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
		if lines := requestLogLines(t, &buf); len(lines) != 0 {
			t.Fatalf("health probe logged: %v", lines)
		}
	})
}

func TestRequestLogRecordsFailedProbesAndDevices(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	mux.HandleFunc("POST /api/v1/attendance/scans", func(w http.ResponseWriter, r *http.Request) {
		NoteActor(r.Context(), "device", "44444444-4444-4444-4444-444444444444")
		_, _ = w.Write([]byte("{}"))
	})
	handler := RequestLog(logger)(mux)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ready", nil))
	lines := requestLogLines(t, &buf)
	if len(lines) != 1 || lines[0]["level"] != "ERROR" || lines[0]["status"] != float64(503) {
		t.Fatalf("failed probe not logged as error: %v", lines)
	}

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/attendance/scans", strings.NewReader(`{"qr":"secret-qr"}`)))
	lines = requestLogLines(t, &buf)
	if len(lines) != 1 || lines[0]["status"] != float64(200) || lines[0]["actor_type"] != "device" || lines[0]["actor_id"] != "44444444-4444-4444-4444-444444444444" {
		t.Fatalf("device request not logged: %v", lines)
	}
}

func TestRequestLogKeepsResponseControllerAndDefaults(t *testing.T) {
	var buf bytes.Buffer
	handler := RequestLog(slog.New(slog.NewJSONHandler(&buf, nil)))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush through request log: %v", err)
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/plain", nil))
	lines := requestLogLines(t, &buf)
	if len(lines) != 1 || lines[0]["status"] != float64(200) || lines[0]["route"] != "unmatched" {
		t.Fatalf("unexpected default log: %v", lines)
	}
	NoteActor(context.Background(), "user", "ignored without RequestLog")
}
