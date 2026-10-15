package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
)

type AttendanceAPI struct {
	service service.AttendanceReports
	timeout time.Duration
}

func NewAttendance(s service.AttendanceReports, t time.Duration) *AttendanceAPI {
	return &AttendanceAPI{s, t}
}
func query(w http.ResponseWriter, r *http.Request) (string, dto.AttendanceQuery, bool) {
	actor, values, limit, offset, ok := reportQuery(w, r, "direction")
	if !ok {
		return "", dto.AttendanceQuery{}, false
	}
	return actor, dto.AttendanceQuery{From: values.Get("from"), To: values.Get("to"), StudentID: values.Get("student_id"), Direction: values.Get("direction"), Limit: limit, Offset: offset}, true
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
