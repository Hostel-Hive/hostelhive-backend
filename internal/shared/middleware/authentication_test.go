package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	identityhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/handler"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type verifyFunc func(context.Context, string) (string, error)

func (f verifyFunc) Verify(ctx context.Context, token string) (string, error) { return f(ctx, token) }

type accountFunc func(context.Context, string) (domain.Account, error)

func (f accountFunc) FindByFirebaseUID(ctx context.Context, uid string) (domain.Account, error) {
	return f(ctx, uid)
}

func TestAuthenticationDecisions(t *testing.T) {
	for _, tc := range []struct {
		name, header, uid, role string
		verifyErr, lookupErr    error
		active                  bool
		want                    int
	}{
		{name: "active", header: "Bearer good", uid: "verified-uid", role: "student", active: true, want: 200},
		{name: "case-insensitive-scheme", header: "bearer good", uid: "verified-uid", role: "warden", active: true, want: 200},
		{name: "missing", want: 401},
		{name: "wrong-scheme", header: "Basic good", want: 401},
		{name: "empty-token", header: "Bearer", want: 401},
		{name: "extra-fields", header: "Bearer good extra", want: 401},
		{name: "invalid", header: "Bearer bad", verifyErr: errors.New("secret invalid token"), want: 401},
		{name: "expired", header: "Bearer expired", verifyErr: errors.New("expired"), want: 401},
		{name: "revoked", header: "Bearer revoked", verifyErr: errors.New("revoked"), want: 401},
		{name: "disabled", header: "Bearer disabled", verifyErr: errors.New("disabled"), want: 401},
		{name: "empty-verified-uid", header: "Bearer good", want: 401},
		{name: "no-local-account", header: "Bearer good", uid: "verified-uid", lookupErr: domain.ErrAccountNotFound, want: 403},
		{name: "inactive", header: "Bearer good", uid: "verified-uid", role: "student", want: 403},
		{name: "unknown-role", header: "Bearer good", uid: "verified-uid", role: "superuser", active: true, want: 403},
		{name: "database-outage", header: "Bearer good", uid: "verified-uid", lookupErr: errors.New("secret database password"), want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookupCalled := false
			verifier := verifyFunc(func(ctx context.Context, raw string) (string, error) {
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("authentication deadline missing")
				}
				return tc.uid, tc.verifyErr
			})
			accounts := accountFunc(func(ctx context.Context, uid string) (domain.Account, error) {
				lookupCalled = true
				if uid != "verified-uid" {
					t.Fatalf("unverified UID: %q", uid)
				}
				return domain.Account{UserID: "local-id", FirebaseUID: uid, Email: "student@example.invalid", Role: tc.role, IsActive: tc.active}, tc.lookupErr
			})
			req := httptest.NewRequest("GET", "/api/v1/me?firebase_uid=attacker&role=admin", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			authentication.Middleware(verifier, accounts, time.Second)(http.HandlerFunc(identityhandler.Me)).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want == 401 && lookupCalled {
				t.Fatal("lookup after failed authentication")
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("unsafe response headers")
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("internal error leaked")
			}
			if tc.want == 200 {
				var account domain.Account
				if err := json.Unmarshal(w.Body.Bytes(), &account); err != nil {
					t.Fatal(err)
				}
				if account.Role != tc.role || account.FirebaseUID != "verified-uid" {
					t.Fatal("client input replaced local identity")
				}
			}
		})
	}
}

func TestFreshAccountState(t *testing.T) {
	account := domain.Account{UserID: "local", FirebaseUID: "uid", Role: "student", IsActive: true}
	calls := 0
	handler := authentication.Middleware(verifyFunc(func(context.Context, string) (string, error) { return "uid", nil }), accountFunc(func(context.Context, string) (domain.Account, error) { calls++; return account, nil }), time.Second)(http.HandlerFunc(identityhandler.Me))
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/me", nil)
		r.Header.Set("Authorization", "Bearer good")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if request().Code != 200 {
		t.Fatal("active account denied")
	}
	account.Role = "warden"
	if !strings.Contains(request().Body.String(), `"role":"warden"`) {
		t.Fatal("stale role")
	}
	account.IsActive = false
	if request().Code != 403 || calls != 3 {
		t.Fatal("deactivation or fresh lookup missing")
	}
}

func TestDuplicateAuthorizationRejected(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/me", nil)
	r.Header.Add("Authorization", "Bearer first")
	r.Header.Add("Authorization", "Bearer second")
	w := httptest.NewRecorder()
	authentication.Middleware(verifyFunc(func(context.Context, string) (string, error) { t.Fatal("ambiguous header verified"); return "", nil }), nil, time.Second)(http.HandlerFunc(identityhandler.Me)).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestAuthenticationCancellation(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if cancelRequest {
			cancel()
		}
		r := httptest.NewRequest("GET", "/api/v1/me", nil).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer good")
		w := httptest.NewRecorder()
		authentication.Middleware(verifyFunc(func(ctx context.Context, _ string) (string, error) { <-ctx.Done(); return "", ctx.Err() }), nil, 10*time.Millisecond)(http.HandlerFunc(identityhandler.Me)).ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
}

func TestAccountLookupTimeout(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/me", nil)
	r.Header.Set("Authorization", "Bearer good")
	w := httptest.NewRecorder()
	authentication.Middleware(verifyFunc(func(context.Context, string) (string, error) { return "uid", nil }), accountFunc(func(ctx context.Context, _ string) (domain.Account, error) {
		<-ctx.Done()
		return domain.Account{}, ctx.Err()
	}), 10*time.Millisecond)(http.HandlerFunc(identityhandler.Me)).ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestMismatchedLocalIdentity(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/me", nil)
	r.Header.Set("Authorization", "Bearer good")
	w := httptest.NewRecorder()
	authentication.Middleware(verifyFunc(func(context.Context, string) (string, error) { return "uid", nil }), accountFunc(func(context.Context, string) (domain.Account, error) {
		return domain.Account{FirebaseUID: "other", Role: "student", IsActive: true}, nil
	}), time.Second)(http.HandlerFunc(identityhandler.Me)).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
