package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type verifyStub struct{}

func (verifyStub) Verify(context.Context, string) (string, error) { return "verified-admin", nil }

type accountStub struct {
	role   string
	active bool
}

func (a accountStub) FindByFirebaseUID(context.Context, string) (domain.Account, error) {
	return domain.Account{FirebaseUID: "verified-admin", Role: a.role, IsActive: a.active}, nil
}

type provisionFunc func(context.Context, string, dto.Input, string) (dto.Result, error)

func (f provisionFunc) Provision(ctx context.Context, key string, input dto.Input, uid string) (dto.Result, error) {
	return f(ctx, key, input, uid)
}

const inputJSON = `{"email":"new.user@example.invalid","role":"student","password":"Local-test-password-20"}`

func TestProvisioningHTTPResponses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		replayed bool
		want     int
	}{
		{"created", nil, false, 201}, {"replayed", nil, true, 200},
		{"validation", domain.ErrProvisionInvalidInput, false, 400}, {"conflict", domain.ErrProvisionConflict, false, 409},
		{"busy", domain.ErrProvisionInProgress, false, 503}, {"unavailable", errors.New("secret password token"), false, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := provisionFunc(func(ctx context.Context, key string, input dto.Input, uid string) (dto.Result, error) {
				if key != "http-test-20" || uid != "verified-admin" || input.Email != "new.user@example.invalid" {
					t.Fatal("unverified identity or invalid request forwarding")
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing provisioning deadline")
				}
				return dto.Result{Account: domain.Account{UserID: "local-id", FirebaseUID: "new-uid", Email: input.Email, Role: input.Role, IsActive: true}, Replayed: tc.replayed}, tc.err
			})
			handler := authentication.Middleware(verifyStub{}, accountStub{domain.RoleAdmin, true}, time.Second)(Handler(service, time.Second))
			r := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(inputJSON))
			r.Header.Set("Authorization", "Bearer id-token")
			r.Header.Set("Idempotency-Key", "http-test-20")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "id-token") {
				t.Fatal("credential leaked")
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("unsafe response headers")
			}
			if tc.err == domain.ErrProvisionInProgress && w.Header().Get("Retry-After") != "1" {
				t.Fatal("missing retry hint")
			}
		})
	}
}

func TestProvisioningHTTPRejectsUntrustedRequests(t *testing.T) {
	for _, tc := range []struct {
		name, role, body, key string
		auth, active          bool
		want                  int
	}{
		{"missing-auth", "", inputJSON, "http-test-20", false, false, 401},
		{"student", domain.RoleStudent, inputJSON, "http-test-20", true, true, 403},
		{"warden", domain.RoleWarden, inputJSON, "http-test-20", true, true, 403},
		{"inactive-admin", domain.RoleAdmin, inputJSON, "http-test-20", true, false, 403},
		{"missing-key", domain.RoleAdmin, inputJSON, "", true, true, 400},
		{"malformed", domain.RoleAdmin, "{", "http-test-20", true, true, 400},
		{"unknown-field", domain.RoleAdmin, `{"email":"user@example.invalid","role":"student","password":"Local-test-password-20","is_active":true}`, "http-test-20", true, true, 400},
		{"extra-json", domain.RoleAdmin, inputJSON + " {}", "http-test-20", true, true, 400},
		{"oversized", domain.RoleAdmin, `{"password":"` + strings.Repeat("x", 17000) + `"}`, "http-test-20", true, true, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := provisionFunc(func(context.Context, string, dto.Input, string) (dto.Result, error) {
				t.Fatal("rejected request reached provisioning")
				return dto.Result{}, nil
			})
			handler := Handler(service, time.Second)
			if tc.auth {
				handler = authentication.Middleware(verifyStub{}, accountStub{tc.role, tc.active}, time.Second)(handler)
			}
			r := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer id-token")
			if tc.key != "" {
				r.Header.Set("Idempotency-Key", tc.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestProvisioningHTTPDeadline(t *testing.T) {
	service := provisionFunc(func(ctx context.Context, _ string, _ dto.Input, _ string) (dto.Result, error) {
		<-ctx.Done()
		return dto.Result{}, ctx.Err()
	})
	handler := authentication.Middleware(verifyStub{}, accountStub{domain.RoleAdmin, true}, time.Second)(Handler(service, 10*time.Millisecond))
	r := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(inputJSON))
	r.Header.Set("Authorization", "Bearer token")
	r.Header.Set("Idempotency-Key", "deadline-20")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestProvisioningRejectsDuplicateIdempotencyHeaders(t *testing.T) {
	service := provisionFunc(func(context.Context, string, dto.Input, string) (dto.Result, error) {
		t.Fatal("ambiguous key reached provisioning")
		return dto.Result{}, nil
	})
	handler := authentication.Middleware(verifyStub{}, accountStub{domain.RoleAdmin, true}, time.Second)(Handler(service, time.Second))
	r := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(inputJSON))
	r.Header.Set("Authorization", "Bearer token")
	r.Header.Add("Idempotency-Key", "request-key-first")
	r.Header.Add("Idempotency-Key", "request-key-second")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
