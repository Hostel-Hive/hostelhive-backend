package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type DeviceService interface {
	Register(context.Context, string, dto.RegisterDevice) (dto.IssuedCredential, error)
	List(context.Context, string, int, int) (domain.DevicePage, error)
	Rotate(context.Context, string, string) (dto.IssuedCredential, error)
	Deactivate(context.Context, string, string) (domain.Device, error)
	Authenticate(context.Context, string) (domain.Device, error)
}

type API struct {
	service              DeviceService
	timeout, authTimeout time.Duration
}

func New(service DeviceService, timeout, authTimeout time.Duration) *API {
	return &API{service, timeout, authTimeout}
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func failure(w http.ResponseWriter, err error) {
	status, code := 503, "scanner_devices_unavailable"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code = 400, "invalid_input"
	case errors.Is(err, domain.ErrUnauthorized):
		status, code = 401, "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, domain.ErrConflict):
		status, code = 409, "scanner_name_conflict"
	}
	response.Error(w, status, code)
}

func admin(w http.ResponseWriter, r *http.Request) (string, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", false
	}
	if !a.IsActive || a.Role != identity.RoleAdmin {
		response.Error(w, 403, "forbidden")
		return "", false
	}
	return a.FirebaseUID, true
}

func empty(w http.ResponseWriter, r *http.Request, queryAllowed bool) bool {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(b) != 0 || (!queryAllowed && r.URL.RawQuery != "") {
		failure(w, domain.ErrInvalid)
		return false
	}
	return true
}

func (a *API) Register(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, domain.ErrInvalid)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2048))
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			response.Error(w, 413, "payload_too_large")
		} else {
			failure(w, domain.ErrInvalid)
		}
		return
	}
	if !utf8.Valid(raw) {
		failure(w, domain.ErrInvalid)
		return
	}
	var in dto.RegisterDevice
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		failure(w, domain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.service.Register(ctx, actor, in)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 201, v)
}

func (a *API) List(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok || !empty(w, r, true) {
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, domain.ErrInvalid)
		return
	}
	limit, offset := 20, 0
	for key, values := range q {
		if len(values) != 1 || (key != "limit" && key != "offset") {
			failure(w, domain.ErrInvalid)
			return
		}
		n, err := strconv.Atoi(values[0])
		if err != nil {
			failure(w, domain.ErrInvalid)
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
	v, err := a.service.List(ctx, actor, limit, offset)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, v)
}

func (a *API) Rotate(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok || !empty(w, r, false) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.service.Rotate(ctx, actor, r.PathValue("deviceID"))
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, v)
}

func (a *API) Deactivate(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok || !empty(w, r, false) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.service.Deactivate(ctx, actor, r.PathValue("deviceID"))
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, v)
}

func (a *API) Check(w http.ResponseWriter, r *http.Request) {
	device, ok := DeviceFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return
	}
	if !empty(w, r, false) {
		return
	}
	reply(w, 200, dto.DeviceCheck{DeviceID: device.DeviceID, Name: device.Name, Status: "authenticated"})
}
