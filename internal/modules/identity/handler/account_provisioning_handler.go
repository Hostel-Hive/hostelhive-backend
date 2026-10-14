package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type Provisioner interface {
	Provision(context.Context, string, dto.Input, string) (dto.Result, error)
}

func Handler(provisioner Provisioner, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Defense in depth if this handler is registered without the route wrapper.
		account, ok := authentication.AccountFromContext(r.Context())
		if !ok {
			response.Error(w, 401, "unauthorized")
			return
		}
		if !account.IsActive || account.Role != domain.RoleAdmin {
			response.Error(w, 403, "forbidden")
			return
		}
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 || !service.ValidRequestKey(keys[0]) {
			response.Error(w, 400, "invalid_input")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input dto.Input
		if err := decoder.Decode(&input); err != nil {
			response.Error(w, 400, "invalid_input")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			response.Error(w, 400, "invalid_input")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result, err := provisioner.Provision(ctx, keys[0], input, account.FirebaseUID)
		switch {
		case errors.Is(err, domain.ErrProvisionInvalidInput):
			response.Error(w, 400, "invalid_input")
		case errors.Is(err, domain.ErrProvisionConflict):
			response.Error(w, 409, "conflict")
		case errors.Is(err, domain.ErrProvisionInProgress):
			w.Header().Set("Retry-After", "1")
			response.Error(w, 503, "request_in_progress")
		case err != nil:
			response.Error(w, 503, "provisioning_unavailable")
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
