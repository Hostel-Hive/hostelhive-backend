package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/inventory/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

func managementError(w http.ResponseWriter, err error) {
	status, code := 503, "inventory_unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code = 400, "invalid_input"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "inventory_conflict"
	case errors.Is(err, domain.ErrForbidden):
		status, code = 403, "forbidden"
	}
	response.Error(w, status, code)
}

func mutate[I, O any](a *API, w http.ResponseWriter, r *http.Request, status int, action func(context.Context, string, I) (O, error)) {
	actor, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return
	}
	if !actor.IsActive || actor.Role != identity.RoleAdmin {
		response.Error(w, 403, "forbidden")
		return
	}
	if r.URL.RawQuery != "" {
		managementError(w, domain.ErrInvalid)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			response.Error(w, 413, "payload_too_large")
		} else {
			managementError(w, domain.ErrInvalid)
		}
		return
	}
	if !utf8.Valid(raw) {
		managementError(w, domain.ErrInvalid)
		return
	}
	var in I
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&in); err != nil {
		managementError(w, domain.ErrInvalid)
		return
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		managementError(w, domain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := action(ctx, actor.FirebaseUID, in)
	if err != nil {
		managementError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}

func (a *API) CreateBlock(w http.ResponseWriter, r *http.Request) {
	mutate(a, w, r, 201, a.service.CreateBlock)
}
func (a *API) UpdateBlock(w http.ResponseWriter, r *http.Request) {
	mutate(a, w, r, 200, func(ctx context.Context, actor string, in dto.BlockDetails) (domain.Block, error) {
		return a.service.UpdateBlock(ctx, actor, r.PathValue("blockID"), in)
	})
}
func (a *API) CreateRoom(w http.ResponseWriter, r *http.Request) {
	mutate(a, w, r, 201, a.service.CreateRoom)
}
func (a *API) UpdateRoom(w http.ResponseWriter, r *http.Request) {
	mutate(a, w, r, 200, func(ctx context.Context, actor string, in dto.RoomDetails) (domain.Room, error) {
		return a.service.UpdateRoom(ctx, actor, r.PathValue("roomID"), in)
	})
}
func (a *API) CreateBed(w http.ResponseWriter, r *http.Request) {
	mutate(a, w, r, 201, a.service.CreateBed)
}
func (a *API) UpdateBed(w http.ResponseWriter, r *http.Request) {
	mutate(a, w, r, 200, func(ctx context.Context, actor string, in dto.BedDetails) (domain.Bed, error) {
		return a.service.UpdateBed(ctx, actor, r.PathValue("bedID"), in)
	})
}
