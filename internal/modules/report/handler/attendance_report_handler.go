package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type AttendanceAPI struct {
	service service.AttendanceReports
	timeout time.Duration
}

func NewAttendance(s service.AttendanceReports, t time.Duration) *AttendanceAPI {
	return &AttendanceAPI{s, t}
}

func query(w http.ResponseWriter, r *http.Request) (string, dto.AttendanceQuery, bool) {
	q := dto.AttendanceQuery{Limit: 20}
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", q, false
	}
	if !a.IsActive || (a.Role != identity.RoleAdmin && a.Role != identity.RoleWarden) {
		response.Error(w, 403, "forbidden")
		return "", q, false
	}
	if r.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1))
		if err != nil || len(raw) > 0 {
			response.Error(w, 400, "invalid_report_query")
			return "", q, false
		}
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		response.Error(w, 400, "invalid_report_query")
		return "", q, false
	}
	for k, v := range values {
		if len(v) != 1 || v[0] == "" {
			response.Error(w, 400, "invalid_report_query")
			return "", q, false
		}
		switch k {
		case "from":
			q.From = v[0]
		case "to":
			q.To = v[0]
		case "student_id":
			q.StudentID = v[0]
		case "direction":
			q.Direction = v[0]
		case "limit", "offset":
			n, err := strconv.Atoi(v[0])
			if err != nil {
				response.Error(w, 400, "invalid_report_query")
				return "", q, false
			}
			if k == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		default:
			response.Error(w, 400, "invalid_report_query")
			return "", q, false
		}
	}
	return a.FirebaseUID, q, true
}
func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		status, code := 503, "service_unavailable"
		if errors.Is(err, domain.ErrInvalid) {
			status, code = 400, "invalid_report_query"
		}
		if errors.Is(err, domain.ErrForbidden) {
			status, code = 403, "forbidden"
		}
		response.Error(w, status, code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
func (a *AttendanceAPI) Movements(w http.ResponseWriter, r *http.Request) {
	actor, q, ok := query(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.service.Movements(ctx, actor, q)
	reply(w, v, err)
}
func (a *AttendanceAPI) Daily(w http.ResponseWriter, r *http.Request) {
	actor, q, ok := query(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.service.Daily(ctx, actor, q)
	reply(w, v, err)
}
