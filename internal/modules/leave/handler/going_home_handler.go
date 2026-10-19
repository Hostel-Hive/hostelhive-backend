package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type GoingHomeAPI struct {
	service service.GoingHomeRepository
	timeout time.Duration
}

func NewGoingHome(s service.GoingHomeRepository, t time.Duration) *GoingHomeAPI {
	return &GoingHomeAPI{s, t}
}
func (a *GoingHomeAPI) Declare(w http.ResponseWriter, r *http.Request) {
	actor, ok := authorized(w, r, true, false)
	if !ok {
		return
	}
	var in dto.GoingHomeInput
	if !decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, replayed, e := a.service.DeclareGoingHome(ctx, actor, in)
	if e != nil {
		status, code := 503, "going_home_unavailable"
		switch {
		case errors.Is(e, domain.ErrInvalid):
			status, code = 400, "invalid_input"
		case errors.Is(e, domain.ErrForbidden):
			status, code = 403, "forbidden"
		case errors.Is(e, domain.ErrIneligible):
			status, code = 409, "active_student_required"
		case errors.Is(e, domain.ErrAlreadyOutside):
			status, code = 409, "already_outside"
		case errors.Is(e, domain.ErrPresenceUnknown):
			status, code = 409, "presence_unknown"
		case errors.Is(e, domain.ErrGoingHomePending):
			status, code = 409, "going_home_pending"
		case errors.Is(e, domain.ErrConflict):
			status, code = 409, "going_home_conflict"
		}
		response.Error(w, status, code)
		return
	}
	status := 201
	if replayed {
		status = 200
	}
	reply(w, status, v)
}
