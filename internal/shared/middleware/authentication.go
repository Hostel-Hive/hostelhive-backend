package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type Verifier interface {
	Verify(context.Context, string) (string, error)
}

type Accounts interface {
	FindByFirebaseUID(context.Context, string) (domain.Account, error)
}

type accountKey struct{}

func AccountFromContext(ctx context.Context) (domain.Account, bool) {
	account, ok := ctx.Value(accountKey{}).(domain.Account)
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
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			fields := strings.Fields(headers[0])
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			uid, err := verifier.Verify(ctx, fields[1])
			if err != nil || uid == "" {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			account, err := accounts.FindByFirebaseUID(ctx, uid)
			if errors.Is(err, domain.ErrAccountNotFound) {
				response.Error(w, http.StatusForbidden, "forbidden")
				return
			}
			if err != nil {
				response.Error(w, http.StatusServiceUnavailable, "service_unavailable")
				return
			}
			if !account.IsActive || account.FirebaseUID != uid || !domain.ValidRole(account.Role) {
				response.Error(w, http.StatusForbidden, "forbidden")
				return
			}
			// The authentication deadline ends before business-handler work begins.
			cancel()
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accountKey{}, account)))
		})
	}
}
