package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/allocation/service"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type API struct {
	service service.Repository
	timeout time.Duration
}

func New(s service.Repository, t time.Duration) *API { return &API{s, t} }
func authorized(w http.ResponseWriter, r *http.Request) (string, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", false
	}
	if !a.IsActive || (a.Role != identity.RoleAdmin && a.Role != identity.RoleWarden) {
		response.Error(w, 403, "forbidden")
		return "", false
	}
	return a.FirebaseUID, true
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, err error) {
	status, code := 503, "allocations_unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code = 400, "invalid_input"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "allocation_conflict"
	case errors.Is(err, domain.ErrIneligible):
		status, code = 409, "active_student_required"
	case errors.Is(err, domain.ErrForbidden):
		status, code = 403, "forbidden"
	}
	response.Error(w, status, code)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err == nil {
		var extra any
		err = d.Decode(&extra)
		if err == io.EOF {
			return true
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		response.Error(w, 413, "payload_too_large")
	} else {
		response.Error(w, 400, "invalid_input")
	}
	return false
}
func parse(raw string) (domain.Filter, error) {
	f := domain.Filter{Limit: 20}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, domain.ErrInvalid
	}
	for k, values := range q {
		if len(values) != 1 || values[0] == "" {
			return f, domain.ErrInvalid
		}
		v := values[0]
		switch k {
		case "student_id":
			f.StudentID = v
		case "bed_id":
			f.BedID = v
		case "active":
			if v != "true" && v != "false" {
				return f, domain.ErrInvalid
			}
			active := v == "true"
			f.Active = &active
		case "limit", "offset":
			n, e := strconv.Atoi(v)
			if e != nil {
				return f, domain.ErrInvalid
			}
			if k == "limit" {
				f.Limit = n
			} else {
				f.Offset = n
			}
		default:
			return f, domain.ErrInvalid
		}
	}
	return f, nil
}
func (a *API) List(w http.ResponseWriter, r *http.Request) {
	actor, ok := authorized(w, r)
	if !ok {
		return
	}
	f, err := parse(r.URL.RawQuery)
	if err != nil {
		failure(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	p, err := a.service.List(ctx, actor, f)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, p)
}
func (a *API) Assign(w http.ResponseWriter, r *http.Request) {
	actor, ok := authorized(w, r)
	if !ok {
		return
	}
	var in dto.Assign
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.service.Assign(ctx, actor, in)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 201, result)
}
func (a *API) Transfer(w http.ResponseWriter, r *http.Request) {
	actor, ok := authorized(w, r)
	if !ok {
		return
	}
	var in dto.Transfer
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.service.Transfer(ctx, actor, r.PathValue("allocationID"), in)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, result)
}
func (a *API) Revoke(w http.ResponseWriter, r *http.Request) {
	actor, ok := authorized(w, r)
	if !ok {
		return
	}
	// No payload is accepted for this action, including chunked bodies.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil || len(body) != 0 {
		failure(w, domain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.service.Revoke(ctx, actor, r.PathValue("allocationID"))
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, result)
}
