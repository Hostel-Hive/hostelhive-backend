package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/service"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

const id = "11111111-1111-4111-8111-111111111111"

type verifier struct{}

func (verifier) Verify(context.Context, string) (string, error) { return "staff", nil }

type account struct {
	role   string
	active bool
}

func (a account) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return identity.Account{FirebaseUID: "staff", Role: a.role, IsActive: a.active}, nil
}

type store struct {
	err      error
	calls    int
	deadline bool
	actor    string
}

func (s *store) record(ctx context.Context, actor string) {
	s.calls++
	_, s.deadline = ctx.Deadline()
	s.actor = actor
}
func (s *store) List(ctx context.Context, actor string, f domain.Filter) (domain.Page, error) {
	s.record(ctx, actor)
	return domain.Page{Items: []domain.Allocation{}, Limit: f.Limit}, s.err
}
func (s *store) Assign(ctx context.Context, actor string, in dto.Assign) (domain.Allocation, error) {
	s.record(ctx, actor)
	return domain.Allocation{AllocationID: id, StudentID: in.StudentID, BedID: in.BedID}, s.err
}
func (s *store) Transfer(ctx context.Context, actor, aid string, in dto.Transfer) (domain.Allocation, error) {
	s.record(ctx, actor)
	return domain.Allocation{AllocationID: aid, BedID: in.BedID}, s.err
}
func (s *store) Revoke(ctx context.Context, actor, aid string) (domain.Allocation, error) {
	s.record(ctx, actor)
	return domain.Allocation{AllocationID: aid}, s.err
}
func TestHTTPContracts(t *testing.T) {
	assign := `{"student_id":"` + id + `","bed_id":"` + id + `"}`
	for _, tc := range []struct {
		name, method, path, body, role, media string
		token, active                         bool
		err                                   error
		want                                  int
	}{
		{"admin assigns", "POST", "/api/v1/allocations", assign, "admin", "application/json", true, true, nil, 201},
		{"warden assigns", "POST", "/api/v1/allocations", assign, "warden", "application/json", true, true, nil, 201},
		{"list", "GET", "/api/v1/allocations?active=true&student_id=" + id + "&bed_id=" + id + "&limit=1&offset=0", "", "warden", "", true, true, nil, 200},
		{"transfer", "POST", "/api/v1/allocations/" + id + "/transfer", `{"bed_id":"` + id + `"}`, "admin", "application/json", true, true, nil, 200},
		{"revoke", "POST", "/api/v1/allocations/" + id + "/revoke", "", "warden", "", true, true, nil, 200},
		{"missing token", "POST", "/api/v1/allocations", assign, "admin", "application/json", false, true, nil, 401},
		{"inactive", "GET", "/api/v1/allocations", "", "admin", "", true, false, nil, 403},
		{"student", "POST", "/api/v1/allocations", assign, "student", "application/json", true, true, nil, 403},
		{"subwarden", "GET", "/api/v1/allocations", "", "sub_warden", "", true, true, nil, 403},
		{"security", "GET", "/api/v1/allocations", "", "security_staff", "", true, true, nil, 403},
		{"bad UUID", "POST", "/api/v1/allocations", `{"student_id":"bad","bed_id":"` + id + `"}`, "admin", "application/json", true, true, nil, 400},
		{"invalid allocation", "POST", "/api/v1/allocations/bad/revoke", "", "admin", "", true, true, nil, 400},
		{"bad transfer", "POST", "/api/v1/allocations/" + id + "/transfer", `{"bed_id":"bad"}`, "admin", "application/json", true, true, nil, 400},
		{"unknown field", "POST", "/api/v1/allocations", `{"actor":"admin"}`, "admin", "application/json", true, true, nil, 400},
		{"null", "POST", "/api/v1/allocations", "null", "admin", "application/json", true, true, nil, 400},
		{"broken JSON", "POST", "/api/v1/allocations", "{", "admin", "application/json", true, true, nil, 400},
		{"trailing JSON", "POST", "/api/v1/allocations", assign + ` {}`, "admin", "application/json", true, true, nil, 400},
		{"body limit", "POST", "/api/v1/allocations", `{"student_id":"` + strings.Repeat("a", 5000) + `"}`, "admin", "application/json", true, true, nil, 413},
		{"wrong media", "POST", "/api/v1/allocations", assign, "admin", "text/plain", true, true, nil, 415},
		{"revoke body", "POST", "/api/v1/allocations/" + id + "/revoke", "{}", "admin", "", true, true, nil, 400},
		{"unknown query", "GET", "/api/v1/allocations?role=admin", "", "admin", "", true, true, nil, 400},
		{"duplicate query", "GET", "/api/v1/allocations?active=true&active=false", "", "admin", "", true, true, nil, 400},
		{"bad boolean", "GET", "/api/v1/allocations?active=1", "", "admin", "", true, true, nil, 400},
		{"bad integer", "GET", "/api/v1/allocations?offset=abc", "", "admin", "", true, true, nil, 400},
		{"bad limit", "GET", "/api/v1/allocations?limit=101", "", "admin", "", true, true, nil, 400},
		{"negative offset", "GET", "/api/v1/allocations?offset=-1", "", "admin", "", true, true, nil, 400},
		{"bad filter UUID", "GET", "/api/v1/allocations?bed_id=bad", "", "admin", "", true, true, nil, 400},
		{"empty query", "GET", "/api/v1/allocations?student_id=", "", "admin", "", true, true, nil, 400},
		{"malformed query", "GET", "/api/v1/allocations?bed_id=%ZZ", "", "admin", "", true, true, nil, 400},
		{"not found", "POST", "/api/v1/allocations", assign, "admin", "application/json", true, true, domain.ErrNotFound, 404},
		{"conflict", "POST", "/api/v1/allocations", assign, "admin", "application/json", true, true, domain.ErrConflict, 409},
		{"ineligible", "POST", "/api/v1/allocations", assign, "admin", "application/json", true, true, domain.ErrIneligible, 409},
		{"changed authority", "GET", "/api/v1/allocations", "", "admin", "", true, true, domain.ErrForbidden, 403},
		{"unavailable", "GET", "/api/v1/allocations", "", "admin", "", true, true, errors.New("private SQL detail"), 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &store{err: tc.err}
			mux := http.NewServeMux()
			allocation.Register(handler.New(service.New(s), time.Second), mux, middleware.Middleware(verifier{}, account{tc.role, tc.active}, time.Second))
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.token {
				r.Header.Set("Authorization", "Bearer token")
			}
			if tc.media != "" {
				r.Header.Set("Content-Type", tc.media)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d %s, want %d", w.Code, w.Body.String(), tc.want)
			}
			if tc.want < 300 && (s.calls != 1 || !s.deadline || s.actor != "staff" || w.Header().Get("Cache-Control") != "no-store") {
				t.Fatal("missing actor/deadline/cache policy")
			}
			if tc.err == nil && tc.want >= 400 && s.calls != 0 {
				t.Fatal("rejected request reached persistence")
			}
			if strings.Contains(w.Body.String(), "private SQL") {
				t.Fatal("private error leaked")
			}
		})
	}
}

func TestDirectHandlerStillRequiresAccount(t *testing.T) {
	a := handler.New(service.New(&store{}), time.Second)
	for _, h := range []http.HandlerFunc{a.List, a.Assign, a.Transfer, a.Revoke} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest("POST", "/api/v1/allocations", nil))
		if w.Code != 401 {
			t.Fatal("direct handler bypassed authentication")
		}
	}
}
