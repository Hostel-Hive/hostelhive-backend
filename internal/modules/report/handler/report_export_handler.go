package handler

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
	"mime"
	"net/http"
	"strconv"
	"time"
)

type ExportAPI struct {
	service     service.ReportExports
	timeout     time.Duration
	slots       chan struct{}
	contentType string
}

func NewExports(s service.ReportExports, t time.Duration) *ExportAPI {
	return &ExportAPI{s, t, make(chan struct{}, 2), domain.ExcelContentType}
}
func (a *ExportAPI) Movements(w http.ResponseWriter, r *http.Request) {
	a.download(w, r, domain.MovementReport, "direction")
}
func (a *ExportAPI) Daily(w http.ResponseWriter, r *http.Request) {
	a.download(w, r, domain.DailyReport, "direction")
}
func (a *ExportAPI) Leaves(w http.ResponseWriter, r *http.Request) {
	a.download(w, r, domain.LeaveReport, "status")
}
func (a *ExportAPI) Complaints(w http.ResponseWriter, r *http.Request) {
	a.download(w, r, domain.ComplaintReport, "category", "status")
}
func (a *ExportAPI) download(w http.ResponseWriter, r *http.Request, kind domain.ReportKind, filters ...string) {
	actor, values, _, _, ok := reportQuery(w, r, filters...)
	if !ok {
		return
	}
	for _, key := range []string{"limit", "offset"} {
		if _, exists := values[key]; exists {
			response.Error(w, 400, "invalid_report_query")
			return
		}
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		response.Error(w, 429, "report_export_busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	file, err := a.service.Export(ctx, actor, kind, dto.ExportQuery{From: values.Get("from"), To: values.Get("to"), StudentID: values.Get("student_id"), Direction: values.Get("direction"), Category: values.Get("category"), Status: values.Get("status")})
	if errors.Is(err, domain.ErrExportTooLarge) {
		response.Error(w, 413, "report_export_too_large")
		return
	}
	if err != nil {
		reply(w, nil, err)
		return
	}
	if ctx.Err() != nil || len(file.Data) == 0 {
		response.Error(w, 503, "service_unavailable")
		return
	}
	// Buffer generation completes before binary headers; failures remain JSON errors.
	w.Header().Set("Content-Type", a.contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	_, _ = w.Write(file.Data)
}
