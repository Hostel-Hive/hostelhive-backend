package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"strconv"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type API struct {
	store   AccountService
	timeout time.Duration
}

type AccountService interface {
	List(context.Context, int, int) (domain.AccountPage, error)
	ChangeRole(context.Context, string, string, string) (domain.Account, error)
	Deactivate(context.Context, string, string) (domain.AccountChange, error)
}

func NewAPI(s AccountService, timeout time.Duration) *API { return &API{s, timeout} }

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func reject(w http.ResponseWriter, status int, code string) {
	if status == 401 {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	reply(w, status, map[string]string{"error": code})
}

func admin(w http.ResponseWriter, r *http.Request) (domain.Account, bool) {
	a, ok := authentication.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return a, false
	}
	if !a.IsActive || a.Role != domain.RoleAdmin {
		reject(w, 403, "forbidden")
		return a, false
	}
	return a, true
}

func failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrManagementNotFound):
		reject(w, 404, "not_found")
	case errors.Is(err, domain.ErrManagementForbidden):
		reject(w, 403, "forbidden")
	case errors.Is(err, domain.ErrLastAdmin):
		reject(w, 409, "last_active_admin")
	default:
		reject(w, 503, "account_management_unavailable")
	}
}

func (a *API) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := admin(w, r); !ok {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		reject(w, 400, "invalid_input")
		return
	}
	limit, offset := 20, 0
	for key, values := range query {
		if len(values) != 1 || (key != "limit" && key != "offset") {
			reject(w, 400, "invalid_input")
			return
		}
		v, e := strconv.Atoi(values[0])
		if e != nil {
			err = e
		}
		if key == "limit" {
			limit = v
		} else {
			offset = v
		}
	}
	if err != nil || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		reject(w, 400, "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	page, err := a.store.List(ctx, limit, offset)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, page)
}

func validRole(role string) bool {
	switch role {
	case domain.RoleAdmin, domain.RoleWarden, domain.RoleSubWarden, domain.RoleSecurityStaff, domain.RoleStudent:
		return true
	}
	return false
}

func (a *API) Role(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("userID")
	if !validation.UUID(id) {
		reject(w, 400, "invalid_input")
		return
	}
	var input dto.RoleInput
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(&struct{}{}) != io.EOF || !validRole(input.Role) {
		reject(w, 400, "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.store.ChangeRole(ctx, actor.FirebaseUID, id, input.Role)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, result)
}

func (a *API) Deactivate(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("userID")
	if !validation.UUID(id) {
		reject(w, 400, "invalid_input")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(body) != 0 {
		reject(w, 400, "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.store.Deactivate(ctx, actor.FirebaseUID, id)
	if err != nil {
		failure(w, err)
		return
	}
	pending := result.RevocationPending
	status := http.StatusOK
	if pending {
		status = http.StatusAccepted
	}
	reply(w, status, result)
}
