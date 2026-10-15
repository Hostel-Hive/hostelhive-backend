package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
)

type LeaveAPI struct {
	service service.LeaveReports
	timeout time.Duration
}

func NewLeaves(s service.LeaveReports, t time.Duration) *LeaveAPI { return &LeaveAPI{s, t} }
func (a *LeaveAPI) Requests(w http.ResponseWriter, r *http.Request) {
	actor, values, limit, offset, ok := reportQuery(w, r, "status")
	if !ok {
		return
	}
	q := dto.LeaveQuery{From: values.Get("from"), To: values.Get("to"), StudentID: values.Get("student_id"), Status: values.Get("status"), Limit: limit, Offset: offset}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, err := a.service.Requests(ctx, actor, q)
	reply(w, v, err)
}
