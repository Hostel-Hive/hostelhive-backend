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
	"strings"
	"time"
	"unicode/utf8"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type API struct {
	store   service.Repository
	timeout time.Duration
}

func New(s service.Repository, t time.Duration) *API { return &API{s, t} }
func reply(w http.ResponseWriter, status int, v any, e error) {
	if e != nil {
		status, code := 503, "faculties_unavailable"
		switch {
		case errors.Is(e, d.ErrInvalid):
			status, code = 400, "invalid_input"
		case errors.Is(e, d.ErrForbidden):
			status, code = 403, "forbidden"
		case errors.Is(e, d.ErrNotFound):
			status, code = 404, "not_found"
		case errors.Is(e, d.ErrConflict):
			status, code = 409, "faculty_name_conflict"
		}
		response.Error(w, status, code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func actor(w http.ResponseWriter, r *http.Request, write bool) (string, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", false
	}
	if !a.IsActive || (a.Role != "admin" && a.Role != "warden") || (write && a.Role != "admin") {
		response.Error(w, 403, "forbidden")
		return "", false
	}
	return a.FirebaseUID, true
}
func empty(w http.ResponseWriter, r *http.Request, query bool) bool {
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if e != nil || len(raw) != 0 || (!query && r.URL.RawQuery != "") {
		reply(w, 0, nil, d.ErrInvalid)
		return false
	}
	return true
}
func details(w http.ResponseWriter, r *http.Request) (dto.Details, bool) {
	var v dto.Details
	if r.URL.RawQuery != "" {
		reply(w, 0, nil, d.ErrInvalid)
		return v, false
	}
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return v, false
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if e != nil {
		response.Error(w, 413, "request_too_large")
		return v, false
	}
	if !utf8.Valid(raw) {
		reply(w, 0, nil, d.ErrInvalid)
		return v, false
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	first, e := dec.Token()
	if e != nil || first != json.Delim('{') {
		reply(w, 0, nil, d.ErrInvalid)
		return v, false
	}
	seen := false
	for dec.More() {
		key, e := dec.Token()
		if e != nil || key != "name" || seen {
			reply(w, 0, nil, d.ErrInvalid)
			return v, false
		}
		seen = true
		if e = dec.Decode(&v.Name); e != nil {
			reply(w, 0, nil, d.ErrInvalid)
			return v, false
		}
	}
	last, e := dec.Token()
	var extra any
	if e != nil || last != json.Delim('}') || dec.Decode(&extra) != io.EOF || !seen {
		reply(w, 0, nil, d.ErrInvalid)
		return v, false
	}
	return v, true
}
func (a *API) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := actor(w, r, false)
	if !ok || !empty(w, r, true) {
		return
	}
	active := true
	f := d.Filter{Limit: 20, Active: &active}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		reply(w, 0, nil, d.ErrInvalid)
		return
	}
	for k, vs := range q {
		if len(vs) != 1 {
			reply(w, 0, nil, d.ErrInvalid)
			return
		}
		switch k {
		case "active":
			switch vs[0] {
			case "true":
				active = true
			case "false":
				active = false
			case "all":
				f.Active = nil
			default:
				reply(w, 0, nil, d.ErrInvalid)
				return
			}
		case "limit", "offset":
			n, e := strconv.Atoi(vs[0])
			if e != nil {
				reply(w, 0, nil, d.ErrInvalid)
				return
			}
			if k == "limit" {
				f.Limit = n
			} else {
				f.Offset = n
			}
		default:
			reply(w, 0, nil, d.ErrInvalid)
			return
		}
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.store.List(ctx, uid, f)
	reply(w, 200, v, e)
}
func (a *API) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := actor(w, r, false)
	if !ok || !empty(w, r, false) {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.store.Get(ctx, uid, r.PathValue("facultyID"))
	reply(w, 200, v, e)
}
func (a *API) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := actor(w, r, true)
	if !ok {
		return
	}
	in, ok := details(w, r)
	if !ok {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.store.Create(ctx, uid, in)
	reply(w, 201, v, e)
}
func (a *API) Rename(w http.ResponseWriter, r *http.Request) {
	uid, ok := actor(w, r, true)
	if !ok {
		return
	}
	in, ok := details(w, r)
	if !ok {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.store.Rename(ctx, uid, r.PathValue("facultyID"), in)
	reply(w, 200, v, e)
}
func (a *API) Activate(w http.ResponseWriter, r *http.Request)   { a.setActive(w, r, true) }
func (a *API) Deactivate(w http.ResponseWriter, r *http.Request) { a.setActive(w, r, false) }
func (a *API) setActive(w http.ResponseWriter, r *http.Request, active bool) {
	uid, ok := actor(w, r, true)
	if !ok || !empty(w, r, false) {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.store.SetActive(ctx, uid, r.PathValue("facultyID"), active)
	reply(w, 200, v, e)
}
