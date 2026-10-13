package handler_test

import (
	"context"
	"errors"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const id = "00000000-0000-4000-8000-000000000001"

type verifier struct{}

func (verifier) Verify(context.Context, string) (string, error) { return "uid", nil }

type accounts struct {
	role   string
	active bool
}

func (a accounts) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return identity.Account{FirebaseUID: "uid", Role: a.role, IsActive: a.active}, nil
}

type store struct {
	actor    string
	calls    int
	err      error
	details  dto.Details
	deadline bool
}

func (s *store) List(ctx context.Context, l, o int) (domain.Page, error) {
	s.calls++
	_, s.deadline = ctx.Deadline()
	return domain.Page{Staff: []domain.Profile{}, Limit: l, Offset: o}, s.err
}
func (s *store) Get(context.Context, string) (domain.Profile, error) {
	s.calls++
	return domain.Profile{UserID: id}, s.err
}
func (s *store) Put(ctx context.Context, actor, target string, d dto.Details) (domain.Profile, error) {
	s.calls++
	_, s.deadline = ctx.Deadline()
	s.details = d
	return domain.Profile{UserID: target, FullName: d.FullName, Designation: d.Designation}, s.err
}
func router(s *store, role string, active bool) *http.ServeMux {
	mux := http.NewServeMux()
	staff.Register(handler.New(service.New(s), time.Second), mux, middleware.Middleware(verifier{}, accounts{role, active}, time.Second))
	return mux
}
func request(mux *http.ServeMux, method, path, body string, token bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token {
		r.Header.Set("Authorization", "Bearer test")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func TestStaffRoleMatrix(t *testing.T) {
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		for _, method := range []string{"GET", "PUT"} {
			s := &store{}
			path := "/api/v1/staff/" + id
			w := request(router(s, role, true), method, path, `{"full_name":"Test","designation":"Warden"}`, true)
			want := 403
			if role == "admin" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("%s %s: %d", role, method, w.Code)
			}
			if want == 403 && s.calls != 0 {
				t.Fatal("unauthorized request reached repository")
			}
		}
		s := &store{}
		w := request(router(s, role, true), "GET", "/api/v1/staff", "", true)
		want := 403
		if role == "admin" {
			want = 200
		}
		if w.Code != want {
			t.Fatal("list role guard")
		}
	}
	s := &store{}
	for _, active := range []bool{true, false} {
		w := request(router(s, "admin", active), "GET", "/api/v1/staff", "", false)
		if w.Code != 401 {
			t.Fatal("missing token accepted")
		}
	}
	if w := request(router(s, "admin", false), "PUT", "/api/v1/staff/"+id, `{}`, true); w.Code != 403 {
		t.Fatal("inactive Admin accepted")
	}
}
func TestStaffValidationAndErrors(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/v1/staff?limit=0", ""}, {"GET", "/api/v1/staff?limit=101", ""}, {"GET", "/api/v1/staff?offset=-1", ""}, {"GET", "/api/v1/staff?offset=100001", ""},
		{"GET", "/api/v1/staff?limit=1&limit=2", ""}, {"GET", "/api/v1/staff?unknown=true", ""}, {"GET", "/api/v1/staff?limit=%zz", ""}, {"GET", "/api/v1/staff/bad", ""},
		{"PUT", "/api/v1/staff/" + id, `{}`}, {"PUT", "/api/v1/staff/" + id, `{"full_name":"Test","designation":"Warden","role":"admin"}`},
		{"PUT", "/api/v1/staff/" + id, `{"full_name":"Test","designation":"Warden"} {}`}, {"PUT", "/api/v1/staff/" + id, `{"full_name":"Test","designation":""}`},
		{"PUT", "/api/v1/staff/" + id, "{\"full_name\":\"" + string([]byte{255}) + "\",\"designation\":\"Warden\"}"},
		{"PUT", "/api/v1/staff/" + id, strings.Repeat("x", 8193)},
	} {
		s := &store{}
		w := request(router(s, "admin", true), tc.method, tc.path, tc.body, true)
		if w.Code != 400 || s.calls != 0 {
			t.Fatalf("bad input %s: %d", tc.path, w.Code)
		}
	}
	for _, tc := range []struct {
		err  error
		code int
	}{{domain.ErrNotFound, 404}, {domain.ErrForbidden, 403}, {domain.ErrStaffRequired, 409}, {errors.New("private credentials"), 503}} {
		w := request(router(&store{err: tc.err}, "admin", true), "GET", "/api/v1/staff/"+id, "", true)
		if w.Code != tc.code || strings.Contains(w.Body.String(), "private") {
			t.Fatal("unsafe error mapping")
		}
	}
	s := &store{}
	w := request(router(s, "admin", true), "PUT", "/api/v1/staff/"+id, `{"full_name":"  Test Staff  ","designation":"  Warden  "}`, true)
	if w.Code != 200 || s.details.FullName != "Test Staff" || s.details.Designation != "Warden" || !s.deadline || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("normalized update %d", w.Code)
	}
	for _, d := range []dto.Details{{FullName: strings.Repeat("界", 201), Designation: "Warden"}, {FullName: "Test", Designation: strings.Repeat("x", 121)}, {FullName: "Test\nBad", Designation: "Warden"}} {
		if _, e := service.New(s).Put(context.Background(), "uid", id, d); !errors.Is(e, domain.ErrInvalid) {
			t.Fatal("invalid details accepted")
		}
	}
}

func (s *store) PutSelf(ctx context.Context, actor string, d dto.SelfDetails) (domain.Profile, error) {
	s.calls++
	s.actor = actor
	_, s.deadline = ctx.Deadline()
	return domain.Profile{UserID: id, FullName: d.FullName, Designation: "Admin-assigned"}, s.err
}
func TestStaffSelfServicePolicy(t *testing.T) {
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		s := &store{}
		w := request(router(s, role, true), "PUT", "/api/v1/staff/me", `{"full_name":"  My Name  "}`, true)
		want := 200
		if role == "student" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("self %s: %d %s", role, w.Code, w.Body.String())
		}
		if want == 200 && (s.calls != 1 || s.actor != "uid" || !s.deadline || !strings.Contains(w.Body.String(), `"full_name":"My Name"`)) {
			t.Fatal("self owner/name/deadline")
		}
		if want == 403 && s.calls != 0 {
			t.Fatal("Student reached self repository")
		}
	}
	for _, body := range []string{`{}`, `{"full_name":"Me","designation":"Admin"}`, `{"full_name":"Me","role":"admin"}`, `{"full_name":"Me","user_id":"` + id + `"}`, `{"full_name":"Me","is_active":true}`, `{"full_name":"Me"} {}`, `{"full_name":""}`, strings.Repeat("x", 8193), "{\"full_name\":\"" + string([]byte{255}) + "\"}"} {
		s := &store{}
		w := request(router(s, "warden", true), "PUT", "/api/v1/staff/me", body, true)
		if w.Code != 400 || s.calls != 0 {
			t.Fatalf("self invalid body accepted: %d", w.Code)
		}
	}
	s := &store{}
	if w := request(router(s, "warden", true), "PUT", "/api/v1/staff/me", `{"full_name":"Me"}`, false); w.Code != 401 {
		t.Fatal("anonymous self update")
	}
	if w := request(router(s, "warden", false), "PUT", "/api/v1/staff/me", `{"full_name":"Me"}`, true); w.Code != 403 {
		t.Fatal("inactive self update")
	}
	s = &store{err: domain.ErrForbidden}
	if w := request(router(s, "warden", true), "PUT", "/api/v1/staff/me", `{"full_name":"Me"}`, true); w.Code != 403 {
		t.Fatal("stale self authority ignored")
	}
}
