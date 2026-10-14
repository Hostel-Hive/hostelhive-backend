package handler

import (
	"context"
	"encoding/json"
	"errors"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type API struct {
	service service.Repository
	timeout time.Duration
}

func New(s service.Repository, t time.Duration) *API { return &API{s, t} }
func authorized(w http.ResponseWriter, r *http.Request, studentOnly, staffOnly bool) (string, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", false
	}
	allowed := a.Role == identity.RoleStudent || a.Role == identity.RoleAdmin || a.Role == identity.RoleWarden
	if !a.IsActive || !allowed || (studentOnly && a.Role != identity.RoleStudent) || (staffOnly && a.Role != identity.RoleAdmin && a.Role != identity.RoleWarden) {
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
	status, code := 503, "leave_unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code = 400, "invalid_input"
	case errors.Is(err, domain.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, domain.ErrIneligible):
		status, code = 409, "active_student_required"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "leave_conflict"
	}
	response.Error(w, status, code)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.URL.RawQuery != "" {
		failure(w, domain.ErrInvalid)
		return false
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err == nil {
		var extra any
		err = d.Decode(&extra)
		if err == io.EOF {
			return true
		}
	}
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		response.Error(w, 413, "payload_too_large")
	} else {
		failure(w, domain.ErrInvalid)
	}
	return false
}
func parse(raw string) (domain.Filter, error) {
	f := domain.Filter{Limit: 20}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, domain.ErrInvalid
	}
	for k, vs := range q {
		if len(vs) != 1 || vs[0] == "" {
			return f, domain.ErrInvalid
		}
		v := vs[0]
		switch k {
		case "student_id":
			f.StudentID = v
		case "status":
			f.Status = v
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
func empty(w http.ResponseWriter, r *http.Request) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil || len(body) > 0 {
		failure(w, domain.ErrInvalid)
		return false
	}
	return true
}
func (a *API) Submit(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, true, false)
	if !ok {
		return
	}
	var in dto.Submit
	if !decode(w, r, &in) {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, err := a.service.Submit(ctx, uid, in)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 201, v)
}
func (a *API) Decide(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, false, true)
	if !ok {
		return
	}
	var in dto.Decision
	if !decode(w, r, &in) {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, err := a.service.Decide(ctx, uid, r.PathValue("leaveID"), in)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, v)
}
func (a *API) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, false, false)
	if !ok {
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, domain.ErrInvalid)
		return
	}
	if !empty(w, r) {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, err := a.service.Get(ctx, uid, r.PathValue("leaveID"))
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, v)
}
func (a *API) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, false, false)
	if !ok {
		return
	}
	if !empty(w, r) {
		return
	}
	f, err := parse(r.URL.RawQuery)
	if err != nil {
		failure(w, err)
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, err := a.service.List(ctx, uid, f)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, v)
}
