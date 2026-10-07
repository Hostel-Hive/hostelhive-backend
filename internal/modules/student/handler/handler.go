package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	sdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	sservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type API struct {
	store   sservice.Repository
	timeout time.Duration
}

func NewAPI(s sservice.Repository, timeout time.Duration) *API { return &API{s, timeout} }

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	if status == 204 {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func reject(w http.ResponseWriter, status int, code string) {
	if status == 401 {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	reply(w, status, map[string]string{"error": code})
}

func failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sdomain.ErrInvalid):
		reject(w, 400, "invalid_input")
	case errors.Is(err, sdomain.ErrNotFound):
		reject(w, 404, "not_found")
	case errors.Is(err, sdomain.ErrConflict):
		reject(w, 409, "duplicate_student")
	case errors.Is(err, sdomain.ErrAccount):
		reject(w, 409, "active_student_account_required")
	case errors.Is(err, sdomain.ErrForbidden):
		reject(w, 403, "forbidden")
	default:
		reject(w, 503, "student_profiles_unavailable")
	}
}

func staff(w http.ResponseWriter, r *http.Request) (domain.Account, bool) {
	a, ok := authentication.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return a, false
	}
	if !a.IsActive || (a.Role != domain.RoleAdmin && a.Role != domain.RoleWarden) {
		reject(w, 403, "forbidden")
		return a, false
	}
	return a, true
}

func parseFilter(raw string) (sdomain.Filter, error) {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return sdomain.Filter{}, sdomain.ErrInvalid
	}
	f := sdomain.Filter{Limit: 20}
	for key, values := range q {
		if len(values) != 1 {
			return f, sdomain.ErrInvalid
		}
		v := strings.TrimSpace(values[0])
		switch key {
		case "limit", "offset", "year":
			n, err := strconv.Atoi(v)
			if err != nil {
				return f, sdomain.ErrInvalid
			}
			switch key {
			case "limit":
				f.Limit = n
			case "offset":
				f.Offset = n
			case "year":
				if n < 1 || n > 10 {
					return f, sdomain.ErrInvalid
				}
				f.Year = n
			}
		case "q":
			if !validation.Text(v, 200) {
				return f, sdomain.ErrInvalid
			}
			f.Q = v
		case "faculty":
			if !validation.Text(v, 120) {
				return f, sdomain.ErrInvalid
			}
			f.Faculty = v
		default:
			return f, sdomain.ErrInvalid
		}
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 1000000 {
		return f, sdomain.ErrInvalid
	}
	return f, nil
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	raw, err := io.ReadAll(r.Body)
	if err != nil || !utf8.Valid(raw) {
		reject(w, 400, "invalid_input")
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		reject(w, 400, "invalid_input")
		return false
	}
	return true
}

func (a *API) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := staff(w, r); !ok {
		return
	}
	f, err := parseFilter(r.URL.RawQuery)
	if err != nil {
		failure(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	page, err := a.store.List(ctx, f)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, page)
}

func (a *API) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := staff(w, r); !ok {
		return
	}
	id := r.PathValue("studentID")
	if !validation.UUID(id) {
		failure(w, sdomain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.store.Get(ctx, id)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, v)
}

func (a *API) Create(w http.ResponseWriter, r *http.Request) {
	actor, ok := staff(w, r)
	if !ok {
		return
	}
	var input sdto.CreateInput
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.store.Create(ctx, actor.FirebaseUID, input)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/students/"+v.StudentID)
	reply(w, 201, v)
}

func (a *API) Update(w http.ResponseWriter, r *http.Request) {
	actor, ok := staff(w, r)
	if !ok {
		return
	}
	id := r.PathValue("studentID")
	if !validation.UUID(id) {
		failure(w, sdomain.ErrInvalid)
		return
	}
	var input sdto.Details
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.store.Update(ctx, actor.FirebaseUID, id, input)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, v)
}

func (a *API) Delete(w http.ResponseWriter, r *http.Request) {
	actor, ok := staff(w, r)
	if !ok {
		return
	}
	id := r.PathValue("studentID")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(body) != 0 || !validation.UUID(id) {
		failure(w, sdomain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	if err = a.store.Delete(ctx, actor.FirebaseUID, id); err != nil {
		failure(w, err)
		return
	}
	reply(w, 204, nil)
}

// Register attaches authentication and the explicit Admin/Warden policy to every route.
