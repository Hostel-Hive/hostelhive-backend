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

type ScanService interface {
	SetDirection(context.Context, string, string, dto.SetDirection) (domain.Device, error)
	Record(context.Context, domain.Device, dto.Scan) (domain.Event, error)
	Presence(context.Context, string, string) (domain.Presence, error)
	Events(context.Context, string, string, int, int) (domain.EventPage, error)
}

type ScanAPI struct {
	service ScanService
	timeout time.Duration
}

func NewScanAPI(service ScanService, timeout time.Duration) *ScanAPI {
	return &ScanAPI{service, timeout}
}

func scanFailure(w http.ResponseWriter, err error) {
	status, code := 503, "attendance_unavailable"
	switch {
	case errors.Is(err, domain.ErrDirection):
		status, code = 409, "scanner_direction_required"
	case errors.Is(err, domain.ErrStudent):
		status, code = 422, "active_student_qr_required"
	case errors.Is(err, domain.ErrEventConflict):
		status, code = 409, "event_id_conflict"
	case errors.Is(err, domain.ErrInvalid):
		status, code = 400, "invalid_input"
	case errors.Is(err, domain.ErrUnauthorized):
		status, code = 401, "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		status, code = 403, "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		status, code = 404, "not_found"
	}
	response.Error(w, status, code)
}

func scanJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.URL.RawQuery != "" {
		scanFailure(w, domain.ErrInvalid)
		return false
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2048))
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			response.Error(w, 413, "payload_too_large")
		} else {
			scanFailure(w, domain.ErrInvalid)
		}
		return false
	}
	if !utf8.Valid(raw) {
		scanFailure(w, domain.ErrInvalid)
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(target) != nil || dec.Decode(&struct{}{}) != io.EOF {
		scanFailure(w, domain.ErrInvalid)
		return false
	}
	return true
}

func (a *ScanAPI) SetDirection(w http.ResponseWriter, r *http.Request) {
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	var in dto.SetDirection
	if !scanJSON(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	d, err := a.service.SetDirection(ctx, actor, r.PathValue("deviceID"), in)
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, d)
}

func (a *ScanAPI) Record(w http.ResponseWriter, r *http.Request) {
	d, ok := DeviceFromContext(r.Context())
	if !ok {
		scanFailure(w, domain.ErrUnauthorized)
		return
	}
	var in dto.Scan
	if !scanJSON(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	e, err := a.service.Record(ctx, d, in)
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, e)
}

func attendanceActor(w http.ResponseWriter, r *http.Request) (string, bool) {
	account, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		scanFailure(w, domain.ErrUnauthorized)
		return "", false
	}
	if !account.IsActive || (account.Role != identity.RoleAdmin && account.Role != identity.RoleWarden) {
		scanFailure(w, domain.ErrForbidden)
		return "", false
	}
	return account.FirebaseUID, true
}

func attendanceQuery(w http.ResponseWriter, r *http.Request, paging bool) (string, int, int, bool) {
	if !empty(w, r, true) {
		return "", 0, 0, false
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		scanFailure(w, domain.ErrInvalid)
		return "", 0, 0, false
	}
	limit, offset := 20, 0
	for key, values := range q {
		if len(values) != 1 || (key != "student_id" && !(paging && (key == "limit" || key == "offset"))) {
			scanFailure(w, domain.ErrInvalid)
			return "", 0, 0, false
		}
		if key == "student_id" {
			continue
		}
		n, err := strconv.Atoi(values[0])
		if err != nil {
			scanFailure(w, domain.ErrInvalid)
			return "", 0, 0, false
		}
		if key == "limit" {
			limit = n
		} else {
			offset = n
		}
	}
	return q.Get("student_id"), limit, offset, true
}

func (a *ScanAPI) Presence(w http.ResponseWriter, r *http.Request) {
	actor, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	id, _, _, ok := attendanceQuery(w, r, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	p, err := a.service.Presence(ctx, actor, id)
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, p)
}

func (a *ScanAPI) Events(w http.ResponseWriter, r *http.Request) {
	actor, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	id, limit, offset, ok := attendanceQuery(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	p, err := a.service.Events(ctx, actor, id, limit, offset)
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, p)
}
