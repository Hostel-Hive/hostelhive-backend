package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"
)

type API struct {
	service service.Repository
	timeout time.Duration
}

func New(s service.Repository, t time.Duration) *API { return &API{s, t} }
func admin(w http.ResponseWriter, r *http.Request) (identity.Account, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return a, false
	}
	if !a.IsActive || a.Role != identity.RoleAdmin {
		response.Error(w, 403, "forbidden")
		return a, false
	}
	return a, true
}
func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalid):
			response.Error(w, 400, "invalid_input")
		case errors.Is(err, domain.ErrNotFound):
			response.Error(w, 404, "not_found")
		case errors.Is(err, domain.ErrForbidden):
			response.Error(w, 403, "forbidden")
		case errors.Is(err, domain.ErrStaffRequired):
			response.Error(w, 409, "staff_account_required")
		default:
			response.Error(w, 503, "staff_profiles_unavailable")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
func (a *API) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := admin(w, r); !ok {
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	limit, offset := 20, 0
	if err != nil {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	for key, vs := range q {
		if len(vs) != 1 || (key != "limit" && key != "offset") {
			reply(w, nil, domain.ErrInvalid)
			return
		}
		n, e := strconv.Atoi(vs[0])
		if e != nil {
			reply(w, nil, domain.ErrInvalid)
			return
		}
		if key == "limit" {
			limit = n
		} else {
			offset = n
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.List(ctx, limit, offset)
	reply(w, v, e)
}
func (a *API) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := admin(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.Get(ctx, r.PathValue("userID"))
	reply(w, v, e)
}
func (a *API) Put(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	var d dto.Details
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil || !utf8.Valid(raw) {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.Put(ctx, actor.FirebaseUID, r.PathValue("userID"), d)
	reply(w, v, e)
}
