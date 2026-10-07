package students

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

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
)

type Repository interface {
	List(context.Context, Filter) (Page, error)
	Get(context.Context, string) (Profile, error)
	Create(context.Context, string, CreateInput) (Profile, error)
	Update(context.Context, string, string, Details) (Profile, error)
	Delete(context.Context, string, string) error
}
type API struct {
	store   Repository
	timeout time.Duration
}

func NewAPI(s Repository, timeout time.Duration) *API { return &API{s, timeout} }
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
	case errors.Is(err, ErrInvalid):
		reject(w, 400, "invalid_input")
	case errors.Is(err, ErrNotFound):
		reject(w, 404, "not_found")
	case errors.Is(err, ErrConflict):
		reject(w, 409, "duplicate_student")
	case errors.Is(err, ErrAccount):
		reject(w, 409, "active_student_account_required")
	case errors.Is(err, ErrForbidden):
		reject(w, 403, "forbidden")
	default:
		reject(w, 503, "student_profiles_unavailable")
	}
}
func admin(w http.ResponseWriter, r *http.Request) (authentication.Account, bool) {
	a, ok := authentication.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return a, false
	}
	if !a.IsActive || a.Role != authentication.RoleAdmin {
		reject(w, 403, "forbidden")
		return a, false
	}
	return a, true
}
func parseFilter(raw string) (Filter, error) {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return Filter{}, ErrInvalid
	}
	f := Filter{Limit: 20}
	for key, values := range q {
		if len(values) != 1 {
			return f, ErrInvalid
		}
		v := strings.TrimSpace(values[0])
		switch key {
		case "limit", "offset", "year":
			n, err := strconv.Atoi(v)
			if err != nil {
				return f, ErrInvalid
			}
			switch key {
			case "limit":
				f.Limit = n
			case "offset":
				f.Offset = n
			case "year":
				if n < 1 || n > 10 {
					return f, ErrInvalid
				}
				f.Year = n
			}
		case "q":
			if !text(v, 200) {
				return f, ErrInvalid
			}
			f.Q = v
		case "faculty":
			if !text(v, 120) {
				return f, ErrInvalid
			}
			f.Faculty = v
		default:
			return f, ErrInvalid
		}
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 1000000 {
		return f, ErrInvalid
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
	if _, ok := admin(w, r); !ok {
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
	if _, ok := admin(w, r); !ok {
		return
	}
	id := r.PathValue("studentID")
	if !uuid.MatchString(id) {
		failure(w, ErrInvalid)
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
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	var input CreateInput
	if !decode(w, r, &input) {
		return
	}
	details, err := Normalize(input.Details)
	if err != nil || !uuid.MatchString(input.UserID) {
		failure(w, ErrInvalid)
		return
	}
	input.Details = details
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
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("studentID")
	if !uuid.MatchString(id) {
		failure(w, ErrInvalid)
		return
	}
	var input Details
	if !decode(w, r, &input) {
		return
	}
	input, err := Normalize(input)
	if err != nil {
		failure(w, err)
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
	actor, ok := admin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("studentID")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(body) != 0 || !uuid.MatchString(id) {
		failure(w, ErrInvalid)
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

// Register attaches authentication and the explicit UC003 admin policy to every route.
func (a *API) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(authentication.RequireRoles(authentication.RoleAdmin)(h))
	}
	mux.Handle("GET /api/v1/students", protect(a.List))
	mux.Handle("GET /api/v1/students/{studentID}", protect(a.Get))
	mux.Handle("POST /api/v1/students", protect(a.Create))
	mux.Handle("PUT /api/v1/students/{studentID}", protect(a.Update))
	mux.Handle("DELETE /api/v1/students/{studentID}", protect(a.Delete))
}
