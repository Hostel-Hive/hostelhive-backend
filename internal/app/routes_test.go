package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOperationalEndpoints(t *testing.T) {
	handler := newHandler(func(context.Context) error { return nil }, time.Second)
	for _, tc := range []struct {
		path   string
		status string
	}{
		{"/health", "ok"},
		{"/ready", "ready"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status code = %d, want 200", response.Code)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON response: %v", err)
			}
			if len(body) != 1 || body["status"] != tc.status {
				t.Fatalf("body = %v, want status %q", body, tc.status)
			}
		})
	}
}

func TestOperationalRouteBoundaries(t *testing.T) {
	handler := newHandler(func(context.Context) error { return nil }, time.Second)
	for _, tc := range []struct {
		method string
		path   string
		code   int
	}{
		{http.MethodGet, "/", http.StatusNotFound},
		{http.MethodGet, "/health/extra", http.StatusNotFound},
		{http.MethodGet, "/ready/extra", http.StatusNotFound},
		{http.MethodPost, "/health", http.StatusMethodNotAllowed},
		{http.MethodPost, "/ready", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.code {
				t.Fatalf("status code = %d, want %d", response.Code, tc.code)
			}
		})
	}
}

func TestReadinessFailureAndRecovery(t *testing.T) {
	dbErr := errors.New("sensitive connection details")
	calls := 0
	handler := newHandler(func(context.Context) error {
		calls++
		return dbErr
	}, time.Second)
	for _, tc := range []struct {
		path string
		code int
		body string
	}{
		{"/ready", http.StatusServiceUnavailable, "{\"status\":\"not_ready\"}\n"},
		{"/health", http.StatusOK, "{\"status\":\"ok\"}\n"},
		{"/ready", http.StatusOK, "{\"status\":\"ready\"}\n"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != tc.code || response.Body.String() != tc.body {
			t.Fatalf("%s: code %d body %q", tc.path, response.Code, response.Body.String())
		}
		if got := response.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		if tc.path == "/health" {
			dbErr = nil
		}
	}
	if calls != 2 {
		t.Fatalf("database checked %d times; /health must not check it", calls)
	}
}

func TestReadinessDeadline(t *testing.T) {
	timeout := 20 * time.Millisecond
	handler := newHandler(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > timeout {
			t.Fatal("database check is missing its bounded deadline")
		}
		<-ctx.Done()
		return ctx.Err()
	}, timeout)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}

func TestReadinessRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handler := newHandler(func(ctx context.Context) error {
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("request cancellation did not reach database check")
		}
		return ctx.Err()
	}, time.Second)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}
