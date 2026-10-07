// Package authentication verifies Firebase identities and current local accounts.
package authentication

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

var ErrAccountNotFound = errors.New("account not found")

type Account struct {
	UserID      string `json:"user_id"`
	FirebaseUID string `json:"firebase_uid"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
}

type Verifier interface {
	Verify(context.Context, string) (string, error)
}
type Accounts interface {
	FindByFirebaseUID(context.Context, string) (Account, error)
}
type accountKey struct{}

func AccountFromContext(ctx context.Context) (Account, bool) {
	account, ok := ctx.Value(accountKey{}).(Account)
	return account, ok
}

// Middleware derives identity only from the verified token and reloads local
// authorization state for every request. Neither query parameters nor claims
// supplied by the client determine the account's role or active status.
func Middleware(verifier Verifier, accounts Accounts, timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			headers := r.Header.Values("Authorization")
			if len(headers) != 1 {
				reject(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			fields := strings.Fields(headers[0])
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
				reject(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			uid, err := verifier.Verify(ctx, fields[1])
			if err != nil || uid == "" {
				reject(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			account, err := accounts.FindByFirebaseUID(ctx, uid)
			if errors.Is(err, ErrAccountNotFound) {
				reject(w, http.StatusForbidden, "forbidden")
				return
			}
			if err != nil {
				reject(w, http.StatusServiceUnavailable, "service_unavailable")
				return
			}
			if !account.IsActive || account.FirebaseUID != uid || !validRole(account.Role) {
				reject(w, http.StatusForbidden, "forbidden")
				return
			}
			// The authentication deadline ends before business-handler work begins.
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accountKey{}, account)))
		})
	}
}

func validRole(role string) bool {
	switch role {
	case RoleAdmin, RoleWarden, RoleSubWarden, RoleSecurityStaff, RoleStudent:
		return true
	}
	return false
}

func reject(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func Me(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFromContext(r.Context())
	if !ok {
		reject(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(account)
}
