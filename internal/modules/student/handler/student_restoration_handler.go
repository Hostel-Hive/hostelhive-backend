package handler

import (
	"context"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"io"
	"net/http"
	"time"
)

type RestorationService interface {
	Restore(context.Context, string, string) (d.RestorationResult, error)
	ListArchived(context.Context, string, d.Filter) (d.Page, error)
}

func (a *RestorationAPI) ListArchived(w http.ResponseWriter, r *http.Request) {
	actor, ok := staff(w, r)
	if !ok {
		return
	}
	if actor.Role != identity.RoleAdmin {
		reject(w, 403, "forbidden")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	filter, filterErr := parseFilter(r.URL.RawQuery)
	if err != nil || len(body) != 0 || filterErr != nil {
		failure(w, d.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	page, err := a.service.ListArchived(ctx, actor.FirebaseUID, filter)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, http.StatusOK, page)
}

type RestorationAPI struct {
	service RestorationService
	timeout time.Duration
}

func NewRestorationAPI(s RestorationService, timeout time.Duration) *RestorationAPI {
	return &RestorationAPI{s, timeout}
}
func (a *RestorationAPI) Restore(w http.ResponseWriter, r *http.Request) {
	actor, ok := staff(w, r)
	if !ok {
		return
	}
	if actor.Role != identity.RoleAdmin {
		reject(w, 403, "forbidden")
		return
	}
	id := r.PathValue("studentID")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(body) != 0 || r.URL.RawQuery != "" || !validation.UUID(id) {
		failure(w, d.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.service.Restore(ctx, actor.FirebaseUID, id)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, http.StatusOK, result)
}
