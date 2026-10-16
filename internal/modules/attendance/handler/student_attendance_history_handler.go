package handler

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type StudentHistoryService interface {
	Events(context.Context, string, dto.StudentHistoryQuery) (domain.StudentHistoryPage, error)
}
type StudentHistoryAPI struct {
	service StudentHistoryService
	timeout time.Duration
}

func NewStudentHistoryAPI(service StudentHistoryService, timeout time.Duration) *StudentHistoryAPI {
	return &StudentHistoryAPI{service, timeout}
}
func studentHistoryQuery(w http.ResponseWriter, r *http.Request) (dto.StudentHistoryQuery, bool) {
	q := dto.StudentHistoryQuery{Limit: 20}
	if !empty(w, r, true) {
		return q, false
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		scanFailure(w, domain.ErrInvalid)
		return q, false
	}
	for key, v := range values {
		if len(v) != 1 || v[0] == "" {
			scanFailure(w, domain.ErrInvalid)
			return q, false
		}
		switch key {
		case "from":
			q.From = v[0]
		case "to":
			q.To = v[0]
		case "direction":
			q.Direction = v[0]
		case "limit", "offset":
			n, err := strconv.Atoi(v[0])
			if err != nil {
				scanFailure(w, domain.ErrInvalid)
				return q, false
			}
			if key == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		default:
			scanFailure(w, domain.ErrInvalid)
			return q, false
		}
	}
	return q, true
}
func (a *StudentHistoryAPI) Events(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		scanFailure(w, domain.ErrUnauthorized)
		return
	}
	if !actor.IsActive || actor.Role != identity.RoleStudent {
		scanFailure(w, domain.ErrForbidden)
		return
	}
	q, ok := studentHistoryQuery(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	page, err := a.service.Events(ctx, actor.FirebaseUID, q)
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, page)
}
