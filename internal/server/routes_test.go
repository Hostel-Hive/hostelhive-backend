package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOperationalEndpoints(t *testing.T) {
	handler := newHandler()
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
	handler := newHandler()
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
