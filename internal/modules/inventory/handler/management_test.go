package handler_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

func TestInventoryManagementHTTP(t *testing.T) {
	const id = "AAAAAAAA-1111-2222-3333-444444444444"
	for _, tc := range []struct {
		name, method, path, body, content, role string
		anonymous, inactive                     bool
		err                                     error
		want                                    int
	}{
		{name: "block create", method: "POST", path: "/blocks", body: `{"name":"  Block A  "}`, want: 201},
		{name: "block rename", method: "PUT", path: "/blocks/" + id, body: `{"name":"Block B"}`, want: 200},
		{name: "room create", method: "POST", path: "/rooms", body: `{"block_id":"` + id + `","room_no":" 101 "}`, want: 201},
		{name: "room rename", method: "PUT", path: "/rooms/" + id, body: `{"room_no":"102"}`, want: 200},
		{name: "bed create", method: "POST", path: "/beds", body: `{"room_id":"` + id + `","bed_no":" 1 "}`, want: 201},
		{name: "bed rename", method: "PUT", path: "/beds/" + id, body: `{"bed_no":"2"}`, want: 200},
		{name: "anonymous", anonymous: true, want: 401},
		{name: "warden", role: identity.RoleWarden, want: 403},
		{name: "student", role: identity.RoleStudent, want: 403},
		{name: "subwarden", role: identity.RoleSubWarden, want: 403},
		{name: "security", role: identity.RoleSecurityStaff, want: 403},
		{name: "inactive", inactive: true, want: 403},
		{name: "empty", body: `{}`, want: 400},
		{name: "null", body: `null`, want: 400},
		{name: "blank", body: `{"name":" \t "}`, want: 400},
		{name: "long name", body: `{"name":"` + strings.Repeat("x", 121) + `"}`, want: 400},
		{name: "long room", path: "/rooms/" + id, method: "PUT", body: `{"room_no":"` + strings.Repeat("x", 33) + `"}`, want: 400},
		{name: "long bed", path: "/beds/" + id, method: "PUT", body: `{"bed_no":"` + strings.Repeat("x", 33) + `"}`, want: 400},
		{name: "bad path", method: "PUT", path: "/blocks/bad", want: 400},
		{name: "missing room parent", path: "/rooms", body: `{"room_no":"101"}`, want: 400},
		{name: "bad bed parent", path: "/beds", body: `{"room_id":"bad","bed_no":"1"}`, want: 400},
		{name: "immutable room parent", method: "PUT", path: "/rooms/" + id, body: `{"room_no":"101","block_id":"` + id + `"}`, want: 400},
		{name: "immutable bed parent", method: "PUT", path: "/beds/" + id, body: `{"bed_no":"1","room_id":"` + id + `"}`, want: 400},
		{name: "actor injection", body: `{"name":"A","actor":"other"}`, want: 400},
		{name: "query", path: "/blocks?name=A", want: 400},
		{name: "malformed", body: `{`, want: 400},
		{name: "trailing document", body: `{"name":"A"} {}`, want: 400},
		{name: "invalid utf8", body: "{\"name\":\"\xff\"}", want: 400},
		{name: "control character", body: `{"name":"A\u0000B"}`, want: 400},
		{name: "wrong content", content: "text/plain", want: 415},
		{name: "oversize", body: strings.Repeat("x", 4097), want: 413},
		{name: "conflict", err: domain.ErrConflict, want: 409},
		{name: "missing", err: domain.ErrNotFound, want: 404},
		{name: "fresh role denied", err: domain.ErrForbidden, want: 403},
		{name: "private error", err: errors.New("secret SQL"), want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.method == "" {
				tc.method = "POST"
			}
			if tc.path == "" {
				tc.path = "/blocks"
			}
			if tc.body == "" {
				tc.body = `{"name":"A"}`
			}
			if tc.content == "" {
				tc.content = "application/json"
			}
			if tc.role == "" {
				tc.role = identity.RoleAdmin
			}
			s := &store{err: tc.err}
			mux := http.NewServeMux()
			inventory.Register(handler.New(service.New(s), time.Second), mux, middleware.Middleware(verifier{}, accounts{tc.role, !tc.inactive}, time.Second))
			r := httptest.NewRequest(tc.method, "/api/v1"+tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.content)
			if !tc.anonymous {
				r.Header.Set("Authorization", "Bearer token")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if tc.err == nil && tc.want >= 400 && s.calls != 0 {
				t.Fatal("invalid request reached persistence")
			}
			if tc.want < 300 {
				if s.calls != 1 || !s.deadline || s.actor != "test" || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("incorrect actor, deadline or cache policy")
				}
				if s.id != "" && s.id != strings.ToLower(id) {
					t.Fatal("identifier not canonicalized")
				}
				if strings.TrimSpace(s.label) != s.label {
					t.Fatal("label not normalized")
				}
			}
			if tc.want == 503 && strings.Contains(w.Body.String(), "secret") {
				t.Fatal("private error leaked")
			}
		})
	}
}
