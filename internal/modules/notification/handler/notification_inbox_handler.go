package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
	"io"
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
func authorized(w http.ResponseWriter, r *http.Request) (string, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", false
	}
	if !a.IsActive || (a.Role != "admin" && a.Role != "warden" && a.Role != "sub_warden" && a.Role != "student") {
		response.Error(w, 403, "forbidden")
		return "", false
	}
	return a.FirebaseUID, true
}
func empty(w http.ResponseWriter, r *http.Request) bool {
	b, e := io.ReadAll(io.LimitReader(r.Body, 1))
	if e != nil || len(b) > 0 {
		response.Error(w, 400, "invalid_input")
		return false
	}
	return true
}
func reply(w http.ResponseWriter, v any, e error) {
	w.Header().Set("Cache-Control", "no-store")
	if e != nil {
		status, code := 503, "notifications_unavailable"
		switch {
		case errors.Is(e, domain.ErrInvalid):
			status, code = 400, "invalid_input"
		case errors.Is(e, domain.ErrForbidden):
			status, code = 403, "forbidden"
		case errors.Is(e, domain.ErrPushConflict):
			status, code = 409, "push_device_conflict"
		case errors.Is(e, domain.ErrNotFound):
			status, code = 404, "not_found"
		}
		response.Error(w, status, code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func parse(raw string) (dto.Filter, error) {
	f := dto.Filter{Limit: 20}
	q, e := url.ParseQuery(raw)
	if e != nil {
		return f, domain.ErrInvalid
	}
	for k, vs := range q {
		if len(vs) != 1 {
			return f, domain.ErrInvalid
		}
		switch k {
		case "limit":
			f.Limit, e = strconv.Atoi(vs[0])
		case "offset":
			f.Offset, e = strconv.Atoi(vs[0])
		case "unread":
			if vs[0] != "true" && vs[0] != "false" {
				return f, domain.ErrInvalid
			}
			f.UnreadOnly = vs[0] == "true"
		default:
			return f, domain.ErrInvalid
		}
		if e != nil {
			return f, domain.ErrInvalid
		}
	}
	return f, nil
}
func (a *API) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r)
	if !ok || !empty(w, r) {
		return
	}
	f, e := parse(r.URL.RawQuery)
	if e != nil {
		reply(w, nil, e)
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.service.List(ctx, uid, f)
	reply(w, v, e)
}
func (a *API) Unread(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r)
	if !ok || !empty(w, r) {
		return
	}
	if r.URL.RawQuery != "" {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.service.Unread(ctx, uid)
	reply(w, v, e)
}
func (a *API) MarkRead(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r)
	if !ok || !empty(w, r) {
		return
	}
	if r.URL.RawQuery != "" {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, e := a.service.MarkRead(ctx, uid, r.PathValue("notificationID"))
	reply(w, v, e)
}
