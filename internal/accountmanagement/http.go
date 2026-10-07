package accountmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
)

type Repository interface {
	List(context.Context, int, int) (Page, error)
	Change(context.Context, string, string, string, bool) (Change, error)
	RetryOne(context.Context, Revoker, string) (bool, error)
	Pending(context.Context, string) (bool, error)
}
type API struct {
	store   Repository
	revoker Revoker
	timeout time.Duration
}

func NewAPI(s Repository, r Revoker, timeout time.Duration) *API { return &API{s, r, timeout} }

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

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
func admin(w http.ResponseWriter, r *http.Request) (authentication.Account, bool) {
	a, ok := authentication.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return a, false
	}
	if !a.IsActive || a.Role != authentication.RoleAdmin {
		reject(w, 403, "forbidden")
		return a, false
	}
	return a, true
}
func failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		reject(w, 404, "not_found")
	case errors.Is(err, ErrForbidden):
		reject(w, 403, "forbidden")
	case errors.Is(err, ErrLastAdmin):
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
	case authentication.RoleAdmin, authentication.RoleWarden, authentication.RoleSubWarden, authentication.RoleSecurityStaff, authentication.RoleStudent:
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
	if !uuid.MatchString(id) {
		reject(w, 400, "invalid_input")
		return
	}
	var input struct {
		Role string `json:"role"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(&struct{}{}) != io.EOF || !validRole(input.Role) {
		reject(w, 400, "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.store.Change(ctx, actor.FirebaseUID, id, input.Role, false)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, result.Account)
}
func (a *API) Deactivate(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("userID")
	if !uuid.MatchString(id) {
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
	result, err := a.store.Change(ctx, actor.FirebaseUID, id, "", true)
	if err != nil {
		failure(w, err)
		return
	}
	// Local access is already blocked and retry work committed. Never claim full
	// success if an external acknowledgement or pending-state check is missing.
	_, _ = a.store.RetryOne(ctx, a.revoker, id)
	pending, err := a.store.Pending(ctx, id)
	if err != nil {
		pending = true
	}
	result.RevocationPending = pending
	status := http.StatusOK
	if pending {
		status = http.StatusAccepted
	}
	reply(w, status, result)
}
