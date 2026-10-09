package handler

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"strings"
	"testing"
	"time"
)

type activationStub struct {
	calls int
	err   error
}

func (f *activationStub) Activate(ctx context.Context, actor, id string) (domain.Account, error) {
	f.calls++
	if _, ok := ctx.Deadline(); !ok || actor != "admin-uid" || id != testID {
		panic("missing identity or deadline")
	}
	return domain.Account{UserID: id, Role: "student", IsActive: true}, f.err
}
func activationRouter(f *activationStub, role string, active bool) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v1/users/{userID}/activate", NewAPI(nil, f, time.Second).Activate)
	return authentication.Middleware(fakeVerifier{}, fakeAccounts{role, active}, time.Second)(m)
}
func TestActivationAccessValidationAndErrors(t *testing.T) {
	path := "/api/v1/users/" + testID + "/activate"
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		f := &activationStub{}
		w := request(activationRouter(f, role, true), "POST", path, "", true)
		want := 403
		if role == "admin" {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("%s %d", role, w.Code)
		}
		if role != "admin" && f.calls != 0 {
			t.Fatal("unauthorized write")
		}
	}
	for _, tc := range []struct {
		path, body    string
		token, active bool
		want          int
	}{{path, "", false, true, 401}, {path, "", true, false, 403}, {path, "{}", true, true, 400}, {"/api/v1/users/bad/activate", "", true, true, 400}} {
		f := &activationStub{}
		w := request(activationRouter(f, "admin", tc.active), "POST", tc.path, tc.body, tc.token)
		if w.Code != tc.want || f.calls != 0 {
			t.Fatalf("status=%d calls=%d", w.Code, f.calls)
		}
	}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{domain.ErrRevocationPending, 409, "revocation_pending"}, {domain.ErrProvisioningIncomplete, 409, "provisioning_incomplete"}, {domain.ErrIdentityMissing, 409, "firebase_identity_missing"}, {domain.ErrIdentityMismatch, 409, "firebase_identity_mismatch"}, {domain.ErrManagementForbidden, 403, "forbidden"}, {domain.ErrManagementNotFound, 404, "not_found"}, {context.DeadlineExceeded, 503, "account_management_unavailable"}} {
		f := &activationStub{err: tc.err}
		w := request(activationRouter(f, "admin", true), "POST", path, "", true)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
}
