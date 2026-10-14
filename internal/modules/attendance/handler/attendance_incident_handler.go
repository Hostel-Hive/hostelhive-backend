package handler

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type IncidentAPI struct {
	service service.IncidentRepository
	timeout time.Duration
}

func NewIncidentAPI(s service.IncidentRepository, t time.Duration) *IncidentAPI {
	return &IncidentAPI{s, t}
}
func (a *IncidentAPI) List(w http.ResponseWriter, r *http.Request) {
	actor, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	if !empty(w, r, true) {
		return
	}
	f := dto.IncidentFilter{Limit: 20}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		scanFailure(w, domain.ErrInvalid)
		return
	}
	for k, vs := range q {
		if len(vs) != 1 || vs[0] == "" {
			scanFailure(w, domain.ErrInvalid)
			return
		}
		switch k {
		case "student_id":
			f.StudentID = vs[0]
		case "direction":
			f.Direction = vs[0]
		case "incident_type":
			f.IncidentType = vs[0]
		case "limit", "offset":
			n, e := strconv.Atoi(vs[0])
			if e != nil {
				scanFailure(w, domain.ErrInvalid)
				return
			}
			if k == "limit" {
				f.Limit = n
			} else {
				f.Offset = n
			}
		default:
			scanFailure(w, domain.ErrInvalid)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	p, err := a.service.Incidents(ctx, actor, f)
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, p)
}
func (a *IncidentAPI) Get(w http.ResponseWriter, r *http.Request) {
	actor, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	if !empty(w, r, false) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.service.Incident(ctx, actor, r.PathValue("incidentID"))
	if err != nil {
		scanFailure(w, err)
		return
	}
	reply(w, 200, v)
}
