package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

const testID = "11111111-1111-1111-1111-111111111111"

type fakeVerifier struct{}

func (fakeVerifier) Verify(context.Context, string) (string, error) { return "admin-uid", nil }

type fakeAccounts struct {
	role   string
	active bool
}

func (f fakeAccounts) FindByFirebaseUID(context.Context, string) (domain.Account, error) {
	return domain.Account{FirebaseUID: "admin-uid", Role: f.role, IsActive: f.active}, nil
}

type fakeStore struct {
	limit, offset, calls int
	pending              bool
	err                  error
	actor, role, id      string
	deactivate           bool
	retryErr             error
}

func (f *fakeStore) List(ctx context.Context, l, o int) (domain.AccountPage, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing operation deadline")
	}
	f.calls++
	f.limit = l
	f.offset = o
	return domain.AccountPage{Users: []domain.Account{{UserID: testID, Email: "test@example.invalid"}}, Limit: l, Offset: o}, f.err
}

func (f *fakeStore) Change(_ context.Context, actor, id, role string, deactivate bool) (domain.AccountChange, error) {
	f.calls++
	f.actor = actor
	f.id = id
	f.role = role
	f.deactivate = deactivate
	return domain.AccountChange{Account: domain.Account{UserID: id, IsActive: !deactivate}}, f.err
}

func (f *fakeStore) RetryOne(context.Context, service.Revoker, string) (bool, error) {
	return true, f.retryErr
}

func (f *fakeStore) Pending(context.Context, string) (bool, error) { return f.pending, f.err }

type fakeRevoker struct {
	fail  bool
	calls int
}

func (f *fakeRevoker) DisableAndRevoke(context.Context, string) error {
	f.calls++
	if f.fail {
		return domain.ErrManagementUnavailable
	}
	return nil
}

func router(s *fakeStore, role string, active bool) http.Handler {
	api := NewAPI(service.NewAccounts(s, &fakeRevoker{}), nil, time.Second)
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/users", api.List)
	m.HandleFunc("PATCH /api/v1/users/{userID}/role", api.Role)
	m.HandleFunc("POST /api/v1/users/{userID}/deactivate", api.Deactivate)
	return authentication.Middleware(fakeVerifier{}, fakeAccounts{role, active}, time.Second)(m)
}

func request(h http.Handler, method, path, body string, token bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token {
		r.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestManagementAccessAndInput(t *testing.T) {
	routes := []struct{ method, path, body string }{{"GET", "/api/v1/users", ""}, {"PATCH", "/api/v1/users/" + testID + "/role", `{"role":"warden"}`}, {"POST", "/api/v1/users/" + testID + "/deactivate", ""}}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		for _, r := range routes {
			s := &fakeStore{}
			w := request(router(s, role, true), r.method, r.path, r.body, true)
			want := 403
			if role == "admin" {
				want = 200
			}
			if w.Code != want || s.calls != map[bool]int{true: 1, false: 0}[role == "admin"] {
				t.Fatalf("%s %s got=%d calls=%d", role, r.path, w.Code, s.calls)
			}
		}
	}
	for _, r := range routes {
		if w := request(router(&fakeStore{}, "admin", true), r.method, r.path, r.body, false); w.Code != 401 {
			t.Fatal("missing token accepted")
		}
		if w := request(router(&fakeStore{}, "admin", false), r.method, r.path, r.body, true); w.Code != 403 {
			t.Fatal("inactive admin accepted")
		}
	}
	for _, r := range []struct{ method, path, body string }{{"GET", "/api/v1/users?limit=0", ""}, {"GET", "/api/v1/users?limit=101", ""}, {"GET", "/api/v1/users?offset=-1", ""}, {"GET", "/api/v1/users?limit=1&limit=2", ""}, {"GET", "/api/v1/users?role=admin", ""}, {"GET", "/api/v1/users?limit=x", ""}, {"GET", "/api/v1/users?offset=1000001", ""}, {"PATCH", "/api/v1/users/bad/role", `{"role":"admin"}`}, {"PATCH", "/api/v1/users/" + testID + "/role", `{"role":"superuser"}`}, {"PATCH", "/api/v1/users/" + testID + "/role", `{"role":"admin","is_active":true}`}, {"PATCH", "/api/v1/users/" + testID + "/role", `{"role":"admin"} {}`}, {"PATCH", "/api/v1/users/" + testID + "/role", strings.Repeat("x", 1025)}, {"POST", "/api/v1/users/" + testID + "/deactivate", `{}`}} {
		s := &fakeStore{}
		w := request(router(s, "admin", true), r.method, r.path, r.body, true)
		if w.Code != 400 || s.calls != 0 {
			t.Fatalf("invalid request %s: %d", r.path, w.Code)
		}
	}
}

func TestManagementResponses(t *testing.T) {
	s := &fakeStore{}
	w := request(router(s, "admin", true), "GET", "/api/v1/users?limit=2&offset=3", "", true)
	if w.Code != 200 || s.limit != 2 || s.offset != 3 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("pagination or response headers incorrect")
	}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		s = &fakeStore{}
		w = request(router(s, "admin", true), "PATCH", "/api/v1/users/"+testID+"/role", `{"role":"`+role+`"}`, true)
		if w.Code != 200 || s.role != role || s.actor != "admin-uid" || s.id != testID || s.deactivate {
			t.Fatal("role change used wrong identity or input")
		}
	}
	for _, tc := range []struct {
		err  error
		code int
	}{{domain.ErrManagementNotFound, 404}, {domain.ErrLastAdmin, 409}, {domain.ErrManagementForbidden, 403}, {errors.New("private database failure"), 503}} {
		s = &fakeStore{err: tc.err}
		w = request(router(s, "admin", true), "PATCH", "/api/v1/users/"+testID+"/role", `{"role":"student"}`, true)
		if w.Code != tc.code || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("unsafe error: %d %s", w.Code, w.Body.String())
		}
	}
	for _, pending := range []bool{false, true} {
		s = &fakeStore{pending: pending, retryErr: domain.ErrManagementUnavailable}
		w = request(router(s, "admin", true), "POST", "/api/v1/users/"+testID+"/deactivate", "", true)
		want := 200
		if pending {
			want = 202
		}
		if w.Code != want || !s.deactivate || s.actor != "admin-uid" {
			t.Fatal("deactivation state or status wrong")
		}
	}
}
