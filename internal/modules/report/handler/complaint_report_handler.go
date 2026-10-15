package handler

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	"net/http"
	"time"
)

type ComplaintAPI struct {
	service service.ComplaintReports
	timeout time.Duration
}

func NewComplaints(s service.ComplaintReports, t time.Duration) *ComplaintAPI {
	return &ComplaintAPI{s, t}
}
func (a *ComplaintAPI) Records(w http.ResponseWriter, r *http.Request) {
	actor, values, limit, offset, ok := reportQuery(w, r, "category", "status")
	if !ok {
		return
	}
	q := dto.ComplaintQuery{From: values.Get("from"), To: values.Get("to"), StudentID: values.Get("student_id"), Category: values.Get("category"), Status: values.Get("status"), Limit: limit, Offset: offset}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.service.Records(ctx, actor, q)
	reply(w, result, err)
}
