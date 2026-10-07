package authentication

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRoleAccessMatrix(t *testing.T) {
	roles := []string{RoleAdmin, RoleWarden, RoleSubWarden, RoleSecurityStaff, RoleStudent}
	policies := []struct {
		name    string
		allowed []string
	}{
		{"admin-only", []string{RoleAdmin}},
		{"wardens", []string{RoleWarden, RoleSubWarden}},
		{"security-only", []string{RoleSecurityStaff}},
		{"student-only", []string{RoleStudent}},
		{"all-human-roles", roles},
	}
	for _, policy := range policies {
		for _, role := range roles {
			t.Run(policy.name+"/"+role, func(t *testing.T) {
				want := http.StatusForbidden
				for _, allowed := range policy.allowed {
					if role == allowed {
						want = http.StatusNoContent
					}
				}
				account := Account{UserID: "local", FirebaseUID: "verified", Role: role, IsActive: true}
				called := false
				handler := Middleware(verifyFunc(func(context.Context, string) (string, error) { return "verified", nil }), accountFunc(func(context.Context, string) (Account, error) { return account, nil }), time.Second)(
					RequireRoles(policy.allowed...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						called = true
						got, ok := AccountFromContext(r.Context())
						if !ok || got != account {
							t.Fatal("verified account was lost")
						}
						w.WriteHeader(http.StatusNoContent)
					})))
				r := httptest.NewRequest("GET", "/example?role=admin", nil)
				r.Header.Set("Authorization", "Bearer verified-token")
				r.Header.Set("X-Role", "admin")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != want || called != (want == http.StatusNoContent) {
					t.Fatalf("status=%d handler_called=%t want=%d", w.Code, called, want)
				}
				if want == http.StatusForbidden {
					if w.Body.String() != "{\"error\":\"forbidden\"}\n" || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
						t.Fatal("unexpected denial response")
					}
				}
			})
		}
	}
}

func TestAuthorizationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account *Account
		roles   []string
		want    int
	}{
		{"no-authentication", nil, []string{RoleStudent}, 401},
		{"inactive", &Account{Role: RoleStudent}, []string{RoleStudent}, 403},
		{"unsupported-account-role", &Account{Role: "superuser", IsActive: true}, []string{RoleStudent}, 403},
		{"empty-policy", &Account{Role: RoleStudent, IsActive: true}, nil, 403},
		{"unsupported-policy", &Account{Role: RoleStudent, IsActive: true}, []string{RoleStudent, "superuser"}, 403},
		{"empty-role-in-policy", &Account{Role: RoleStudent, IsActive: true}, []string{RoleStudent, ""}, 403},
		{"case-sensitive-policy", &Account{Role: RoleStudent, IsActive: true}, []string{"Student"}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/example", nil)
			r.Header.Set("Authorization", "Bearer unverified")
			if tc.account != nil {
				r = r.WithContext(context.WithValue(r.Context(), accountKey{}, *tc.account))
			}
			w := httptest.NewRecorder()
			RequireRoles(tc.roles...)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("denied request reached business handler") })).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d want %d", w.Code, tc.want)
			}
			if tc.want == 401 && w.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing challenge")
			}
		})
	}
}

func TestAuthorizationPolicyCopied(t *testing.T) {
	roles := []string{RoleStudent, RoleStudent}
	handler := RequireRoles(roles...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	roles[0] = RoleAdmin
	roles[1] = RoleAdmin
	for _, tc := range []struct {
		role string
		want int
	}{{RoleStudent, 204}, {RoleAdmin, 403}} {
		r := httptest.NewRequest("GET", "/example", nil).WithContext(context.WithValue(context.Background(), accountKey{}, Account{Role: tc.role, IsActive: true}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("mutable policy: role=%s code=%d", tc.role, w.Code)
		}
	}
}

func TestAuthorizationUsesFreshLocalState(t *testing.T) {
	account := Account{UserID: "local", FirebaseUID: "verified", Role: RoleStudent, IsActive: true}
	lookups := 0
	handler := Middleware(verifyFunc(func(context.Context, string) (string, error) { return "verified", nil }), accountFunc(func(context.Context, string) (Account, error) { lookups++; return account, nil }), time.Second)(
		RequireRoles(RoleStudent)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	request := func(want int) {
		t.Helper()
		r := httptest.NewRequest("GET", "/example", nil)
		r.Header.Set("Authorization", "Bearer same-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("got %d want %d", w.Code, want)
		}
	}
	request(204)
	account.Role = RoleAdmin
	request(403)
	account.Role = RoleStudent
	request(204)
	account.IsActive = false
	request(403)
	if lookups != 4 {
		t.Fatal("authorization used stale account state")
	}
}

func TestFailedAuthenticationNeverReachesAuthorizationHandler(t *testing.T) {
	for _, withHeader := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/example", nil)
		if withHeader {
			r.Header.Set("Authorization", "Bearer invalid-token")
		}
		w := httptest.NewRecorder()
		handler := Middleware(verifyFunc(func(context.Context, string) (string, error) { return "", errors.New("invalid token") }), accountFunc(func(context.Context, string) (Account, error) {
			t.Fatal("lookup after failed verification")
			return Account{}, nil
		}), time.Second)(
			RequireRoles(RoleAdmin)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("failed authentication reached business handler") })))
		handler.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
}
