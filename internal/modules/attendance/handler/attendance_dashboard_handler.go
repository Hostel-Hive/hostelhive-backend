package handler

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
)

type DashboardService interface {
	PresenceStudents(context.Context, string, dto.PresenceFilter) (domain.PresencePage, error)
	PresenceSummary(context.Context, string, string) (domain.PresenceSummary, error)
}

type DashboardAPI struct {
	service DashboardService
	timeout time.Duration
}

func NewDashboardAPI(service DashboardService, timeout time.Duration) *DashboardAPI {
	return &DashboardAPI{service, timeout}
}

func dashboardQuery(w http.ResponseWriter, r *http.Request, listing bool) (dto.PresenceFilter, bool) {
	filter := dto.PresenceFilter{Limit: 20}
	if !empty(w, r, true) {
		return filter, false
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		scanFailure(w, domain.ErrInvalid)
		return filter, false
	}
	for key, values := range query {
		if len(values) != 1 || (key != "q" && !(listing && (key == "status" || key == "limit" || key == "offset"))) {
			scanFailure(w, domain.ErrInvalid)
			return filter, false
		}
		switch key {
		case "q":
			filter.Query = values[0]
		case "status":
			if values[0] == "" {
				scanFailure(w, domain.ErrInvalid)
				return filter, false
			}
			filter.Status = values[0]
		default:
			n, err := strconv.Atoi(values[0])
			if err != nil {
				scanFailure(w, domain.ErrInvalid)
				return filter, false
			}
			if key == "limit" {
				filter.Limit = n
			} else {
				filter.Offset = n
			}
		}
	}
	return filter, true
}

func (a *DashboardAPI) Students(w http.ResponseWriter, r *http.Request) {
	actor, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	filter, ok := dashboardQuery(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	page, err := a.service.PresenceStudents(ctx, actor, filter)
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, page)
}

func (a *DashboardAPI) Summary(w http.ResponseWriter, r *http.Request) {
	actor, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	filter, ok := dashboardQuery(w, r, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	summary, err := a.service.PresenceSummary(ctx, actor, filter.Query)
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, summary)
}
