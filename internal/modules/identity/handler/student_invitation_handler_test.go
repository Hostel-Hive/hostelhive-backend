package handler_test

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/handler"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type inviteVerify struct{}

func (inviteVerify) Verify(context.Context, string) (string, error) { return "actor", nil }

type inviteAccounts struct {
	role   string
	active bool
}

func (a inviteAccounts) FindByFirebaseUID(context.Context, string) (d.Account, error) {
	return d.Account{FirebaseUID: "actor", Role: a.role, IsActive: a.active}, nil
}

type invitationHTTPFake struct {
	calls  int
	err    error
	replay bool
}

func (f *invitationHTTPFake) called(ctx context.Context) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	f.calls++
}
func (f *invitationHTTPFake) Register(ctx context.Context, actor, ip string, v dto.StudentInvitationInput) (d.StudentInvitation, bool, error) {
	f.called(ctx)
	if actor != "actor" || ip != "192.0.2.1" {
		panic("untrusted actor or IP")
	}
	return d.StudentInvitation{}, f.replay, f.err
}
func (f *invitationHTTPFake) Get(ctx context.Context, _, _ string) (d.StudentInvitation, error) {
	f.called(ctx)
	return d.StudentInvitation{}, f.err
}
func (f *invitationHTTPFake) List(ctx context.Context, _ string, _, _ int) (d.InvitationPage, error) {
	f.called(ctx)
	return d.InvitationPage{}, f.err
}
func (f *invitationHTTPFake) Resend(ctx context.Context, _, _, _, _ string) (d.StudentInvitation, error) {
	f.called(ctx)
	return d.StudentInvitation{}, f.err
}
func (f *invitationHTTPFake) Complete(ctx context.Context, _, token string) (d.SetupResult, error) {
	f.called(ctx)
	if token != "token" {
		panic("missing token")
	}
	return d.SetupResult{}, f.err
}
func (f *invitationHTTPFake) Import(ctx context.Context, _, _ string, _ []byte) (svc.InvitationImportReport, error) {
	f.called(ctx)
	return svc.InvitationImportReport{}, f.err
}
func invitationRequest(f *invitationHTTPFake, role string, active, token bool, method, path, body, media string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	identity.RegisterInvitations(h.NewInvitationAPI(f, time.Second), mux, m.Middleware(inviteVerify{}, inviteAccounts{role, active}, time.Second))
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", media)
	if token {
		r.Header.Set("Authorization", "Bearer token")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func TestInvitationHTTPRoutesAndRoles(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	for _, endpoint := range []struct {
		method, path, body, media, role string
		status                          int
	}{
		{"POST", "/api/v1/student-invitations", "{}", "application/json", "admin", 201},
		{"GET", "/api/v1/student-invitations", "", "", "admin", 200},
		{"GET", "/api/v1/student-invitations/" + id, "", "", "admin", 200},
		{"POST", "/api/v1/student-invitations/" + id + "/resend", "{}", "application/json", "admin", 200},
		{"POST", "/api/v1/student-invitations/import", "csv", "text/csv", "admin", 200},
		{"GET", "/api/v1/student-invitations/import/template", "", "", "admin", 200},
		{"POST", "/api/v1/me/setup/complete", "", "", "student", 200},
	} {
		for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
			f := &invitationHTTPFake{}
			w := invitationRequest(f, role, true, true, endpoint.method, endpoint.path, endpoint.body, endpoint.media)
			want := 403
			if role == endpoint.role {
				want = endpoint.status
			}
			if w.Code != want {
				t.Fatalf("%s %s %d: %s", role, endpoint.path, w.Code, w.Body.String())
			}
			if want == 403 && f.calls != 0 {
				t.Fatal("forbidden service call")
			}
		}
		for _, tc := range []struct {
			active, token bool
			status        int
		}{{true, false, 401}, {false, true, 403}} {
			f := &invitationHTTPFake{}
			if w := invitationRequest(f, endpoint.role, tc.active, tc.token, endpoint.method, endpoint.path, endpoint.body, endpoint.media); w.Code != tc.status || f.calls != 0 {
				t.Fatal("authentication", w.Code)
			}
		}
	}
	for _, tc := range []struct {
		method, path, body, media string
		status                    int
	}{
		{"POST", "/api/v1/student-invitations", `{"password":"private"}`, "application/json", 400},
		{"POST", "/api/v1/student-invitations", "{} {}", "application/json", 400},
		{"POST", "/api/v1/student-invitations", "{}", "text/plain", 415},
		{"POST", "/api/v1/student-invitations?role=admin", "{}", "application/json", 400},
		{"GET", "/api/v1/student-invitations?limit=1&limit=2", "", "", 400},
		{"GET", "/api/v1/student-invitations?unknown=1", "", "", 400},
		{"GET", "/api/v1/student-invitations?offset=bad", "", "", 400},
		{"GET", "/api/v1/student-invitations", "{}", "application/json", 400},
		{"POST", "/api/v1/student-invitations/import", "csv", "text/plain", 415},
		{"POST", "/api/v1/student-invitations/import", "csv", "text/csv; charset=latin1", 415},
	} {
		f := &invitationHTTPFake{}
		w := invitationRequest(f, "admin", true, true, tc.method, tc.path, tc.body, tc.media)
		if w.Code != tc.status || f.calls != 0 {
			t.Fatalf("bad request %s %d", tc.path, w.Code)
		}
	}
	if w := invitationRequest(&invitationHTTPFake{}, "student", true, true, "POST", "/api/v1/me/setup/complete", "{}", "application/json"); w.Code != 400 {
		t.Fatal("client completion flag/body accepted")
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{d.ErrInvitationInvalid, 400}, {d.ErrInvitationConflict, 409}, {d.ErrInvitationForbidden, 403}, {d.ErrInvitationNotFound, 404}, {d.ErrInvitationRateLimited, 429}, {d.ErrSetupPending, 409}, {errors.New("private action token"), 503}} {
		w := invitationRequest(&invitationHTTPFake{err: tc.err}, "admin", true, true, "POST", "/api/v1/student-invitations", "{}", "application/json")
		if w.Code != tc.status || strings.Contains(w.Body.String(), "private") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unsafe error", w.Code, w.Body.String())
		}
	}
}
