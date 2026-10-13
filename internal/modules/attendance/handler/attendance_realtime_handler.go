package handler

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"net/http"
	"time"
)

type RealtimeService interface {
	Session(context.Context, string) (domain.RealtimeSession, error)
	Status(context.Context, string) (domain.PublicationStatus, error)
}
type RealtimeAPI struct {
	service RealtimeService
	timeout time.Duration
}

func NewRealtimeAPI(s RealtimeService, timeout time.Duration) *RealtimeAPI {
	return &RealtimeAPI{s, timeout}
}

func (a *RealtimeAPI) handle(w http.ResponseWriter, r *http.Request, session bool) {
	actor, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	if !empty(w, r, true) {
		return
	}
	if r.URL.RawQuery != "" {
		scanFailure(w, domain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	if session {
		result, err := a.service.Session(ctx, actor)
		if err != nil {
			scanFailure(w, err)
			return
		}
		reply(w, 200, result)
	} else {
		result, err := a.service.Status(ctx, actor)
		if err != nil {
			scanFailure(w, err)
			return
		}
		reply(w, 200, result)
	}
}
func (a *RealtimeAPI) Session(w http.ResponseWriter, r *http.Request) { a.handle(w, r, true) }
func (a *RealtimeAPI) Status(w http.ResponseWriter, r *http.Request)  { a.handle(w, r, false) }
