package provisioning

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
)

type Provisioner interface {
	Provision(context.Context, string, Input, string) (Result, error)
}

func Handler(service Provisioner, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Defense in depth if this handler is registered without the route wrapper.
		account, ok := authentication.AccountFromContext(r.Context())
		if !ok {
			writeError(w, 401, "unauthorized")
			return
		}
		if !account.IsActive || account.Role != authentication.RoleAdmin {
			writeError(w, 403, "forbidden")
			return
		}
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 || !keyPattern.MatchString(keys[0]) {
			writeError(w, 400, "invalid_input")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input Input
		if err := decoder.Decode(&input); err != nil {
			writeError(w, 400, "invalid_input")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeError(w, 400, "invalid_input")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result, err := service.Provision(ctx, keys[0], input, account.FirebaseUID)
		switch {
		case errors.Is(err, ErrInvalidInput):
			writeError(w, 400, "invalid_input")
		case errors.Is(err, ErrConflict):
			writeError(w, 409, "conflict")
		case errors.Is(err, ErrInProgress):
			w.Header().Set("Retry-After", "1")
			writeError(w, 503, "request_in_progress")
		case err != nil:
			writeError(w, 503, "provisioning_unavailable")
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			if result.Replayed {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusCreated)
			}
			_ = json.NewEncoder(w).Encode(result)
		}
	})
}
func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status == 401 {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
