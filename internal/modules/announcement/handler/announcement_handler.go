package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/announcement/service"
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
func authorized(w http.ResponseWriter, r *http.Request, staff bool) (string, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", false
	}
	if !a.IsActive || (a.Role != "admin" && a.Role != "warden" && (staff || a.Role != "student")) {
		response.Error(w, 403, "forbidden")
		return "", false
	}
	return a.FirebaseUID, true
}
func reply(w http.ResponseWriter, status int, v any, e error) {
	w.Header().Set("Cache-Control", "no-store")
	if e != nil {
		code := "announcements_unavailable"
		status = 503
		switch {
		case errors.Is(e, domain.ErrInvalid):
			status, code = 400, "invalid_input"
		case errors.Is(e, domain.ErrForbidden):
			status, code = 403, "forbidden"
		case errors.Is(e, domain.ErrNotFound):
			status, code = 404, "not_found"
		case errors.Is(e, domain.ErrConflict):
			status, code = 409, "announcement_conflict"
		}
		response.Error(w, status, code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func empty(w http.ResponseWriter, r *http.Request, noQuery bool) bool {
	b, e := io.ReadAll(io.LimitReader(r.Body, 1))
	if e != nil || len(b) > 0 || (noQuery && r.URL.RawQuery != "") {
		reply(w, 0, nil, domain.ErrInvalid)
		return false
	}
	return true
}
func (a *API) Publish(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, true)
	if !ok {
		return
	}
	if r.URL.RawQuery != "" {
		reply(w, 0, nil, domain.ErrInvalid)
		return
	}
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var in dto.Publish
	e = decoder.Decode(&in)
	if e == nil {
		e = decoder.Decode(&struct{}{})
		if e == io.EOF {
			e = nil
		} else if e == nil {
			e = domain.ErrInvalid
		}
	}
	if e != nil {
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			response.Error(w, 413, "payload_too_large")
		} else {
			reply(w, 0, nil, domain.ErrInvalid)
		}
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.service.Publish(ctx, uid, in)
	status := 201
	if v.Replayed {
		status = 200
	}
	reply(w, status, v, e)
}
func (a *API) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, false)
	if !ok || !empty(w, r, false) {
		return
	}
	f := dto.Filter{Limit: 20}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		reply(w, 0, nil, domain.ErrInvalid)
		return
	}
	for k, vs := range q {
		if len(vs) != 1 {
			reply(w, 0, nil, domain.ErrInvalid)
			return
		}
		n, e := strconv.Atoi(vs[0])
		if e != nil {
			reply(w, 0, nil, domain.ErrInvalid)
			return
		}
		switch k {
		case "limit":
			f.Limit = n
		case "offset":
			f.Offset = n
		default:
			reply(w, 0, nil, domain.ErrInvalid)
			return
		}
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.service.List(ctx, uid, f)
	reply(w, 200, v, e)
}
func (a *API) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, false)
	if !ok || !empty(w, r, true) {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.service.Get(ctx, uid, r.PathValue("announcementID"))
	reply(w, 200, v, e)
}
func (a *API) Summary(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, true)
	if !ok || !empty(w, r, true) {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.service.Summary(ctx, uid, r.PathValue("announcementID"))
	reply(w, 200, v, e)
}
