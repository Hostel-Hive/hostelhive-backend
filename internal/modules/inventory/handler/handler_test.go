package handler_test

import (
	"context"
	"errors"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type verifier struct{}

func (verifier) Verify(context.Context, string) (string, error) { return "test", nil }

type accounts struct {
	role   string
	active bool
}

func (a accounts) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return identity.Account{FirebaseUID: "test", Role: a.role, IsActive: a.active}, nil
}

type store struct {
	err      error
	calls    int
	deadline bool
}

func (s *store) Blocks(ctx context.Context, f domain.Filter) (domain.Page[domain.Block], error) {
	s.calls++
	_, s.deadline = ctx.Deadline()
	return domain.Page[domain.Block]{Items: []domain.Block{}, Limit: f.Limit, Offset: f.Offset}, s.err
}
func (s *store) Rooms(context.Context, domain.Filter) (domain.Page[domain.Room], error) {
	s.calls++
	return domain.Page[domain.Room]{Items: []domain.Room{}}, s.err
}
func (s *store) Beds(context.Context, domain.Filter) (domain.Page[domain.Bed], error) {
	s.calls++
	return domain.Page[domain.Bed]{Items: []domain.Bed{}}, s.err
}
func TestInventoryHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, path, token string
		role              string
		active            bool
		err               error
		want              int
	}{
		{"warden", "/api/v1/blocks", "token", identity.RoleWarden, true, nil, 200},
		{"anonymous", "/api/v1/beds", "", identity.RoleWarden, true, nil, 401},
		{"admin", "/api/v1/rooms", "token", identity.RoleAdmin, true, nil, 403},
		{"student", "/api/v1/beds", "token", identity.RoleStudent, true, nil, 403},
		{"subwarden", "/api/v1/blocks", "token", identity.RoleSubWarden, true, nil, 403},
		{"security", "/api/v1/blocks", "token", identity.RoleSecurityStaff, true, nil, 403},
		{"inactive", "/api/v1/blocks", "token", identity.RoleWarden, false, nil, 403},
		{"unavailable", "/api/v1/blocks", "token", identity.RoleWarden, true, errors.New("private SQL detail"), 503},
		{"limit", "/api/v1/blocks?limit=101", "token", identity.RoleWarden, true, nil, 400},
		{"offset", "/api/v1/blocks?offset=-1", "token", identity.RoleWarden, true, nil, 400},
		{"duplicate", "/api/v1/blocks?limit=1&limit=2", "token", identity.RoleWarden, true, nil, 400},
		{"unknown", "/api/v1/beds?role=admin", "token", identity.RoleWarden, true, nil, 400},
		{"uuid", "/api/v1/rooms?block_id=bad", "token", identity.RoleWarden, true, nil, 400},
		{"empty", "/api/v1/rooms?block_id=", "token", identity.RoleWarden, true, nil, 400},
		{"unsupported", "/api/v1/blocks?available=true", "token", identity.RoleWarden, true, nil, 400},
		{"boolean", "/api/v1/beds?available=1", "token", identity.RoleWarden, true, nil, 400},
		{"malformed", "/api/v1/beds?room_id=%ZZ", "token", identity.RoleWarden, true, nil, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &store{err: tc.err}
			mux := http.NewServeMux()
			inventory.Register(handler.New(service.New(s), time.Second), mux, middleware.Middleware(verifier{}, accounts{tc.role, tc.active}, time.Second))
			r := httptest.NewRequest("GET", tc.path, nil)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if tc.want == 400 || tc.want == 401 || tc.want == 403 {
				if s.calls != 0 {
					t.Fatal("rejected request reached database")
				}
			}
			if tc.want == 200 {
				if !s.deadline || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("missing deadline/cache protection")
				}
			}
			if tc.want == 503 && w.Body.String() != "{\"error\":\"inventory_unavailable\"}\n" {
				t.Fatal("leaked database error")
			}
		})
	}
}
