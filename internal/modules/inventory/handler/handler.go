package handler

import (
	"context"
	"encoding/json"
	"errors"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
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
func parse(raw string) (domain.Filter, error) {
	f := domain.Filter{Limit: 20}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, domain.ErrInvalid
	}
	for k, vs := range q {
		if len(vs) != 1 {
			return f, domain.ErrInvalid
		}
		v := vs[0]
		switch k {
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
		case "block_id":
			f.BlockID = v
			if v == "" {
				return f, domain.ErrInvalid
			}
		case "room_id":
			f.RoomID = v
			if v == "" {
				return f, domain.ErrInvalid
			}
		case "available":
			if v != "true" && v != "false" {
				return f, domain.ErrInvalid
			}
			b := v == "true"
			f.Available = &b
		default:
			return f, domain.ErrInvalid
		}
	}
	return f, nil
}
func list[T any](a *API, w http.ResponseWriter, r *http.Request, query func(context.Context, domain.Filter) (domain.Page[T], error)) {
	account, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return
	}
	if !account.IsActive || account.Role != identity.RoleWarden {
		response.Error(w, 403, "forbidden")
		return
	}
	f, err := parse(r.URL.RawQuery)
	if err != nil {
		response.Error(w, 400, "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := query(ctx, f)
	if errors.Is(err, domain.ErrInvalid) {
		response.Error(w, 400, "invalid_input")
		return
	}
	if err != nil {
		response.Error(w, 503, "inventory_unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(result)
}
func (a *API) Blocks(w http.ResponseWriter, r *http.Request) { list(a, w, r, a.service.Blocks) }
func (a *API) Rooms(w http.ResponseWriter, r *http.Request)  { list(a, w, r, a.service.Rooms) }
func (a *API) Beds(w http.ResponseWriter, r *http.Request)   { list(a, w, r, a.service.Beds) }
