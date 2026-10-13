package handler

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	domain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	service "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type ImportService interface {
	Import(context.Context, string, []byte) (domain.ImportReport, error)
}
type ImportAPI struct {
	service ImportService
	timeout time.Duration
}

func NewImportAPI(s ImportService, timeout time.Duration) *ImportAPI { return &ImportAPI{s, timeout} }
func importAdmin(w http.ResponseWriter, r *http.Request) (identity.Account, bool) {
	actor, ok := authentication.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return actor, false
	}
	if !actor.IsActive || actor.Role != identity.RoleAdmin {
		reject(w, 403, "forbidden")
		return actor, false
	}
	return actor, true
}
func (a *ImportAPI) Template(w http.ResponseWriter, r *http.Request) {
	if _, ok := importAdmin(w, r); !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="student-import-template.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, service.CSVHeader+"\r\n")
}
func (a *ImportAPI) Import(w http.ResponseWriter, r *http.Request) {
	actor, ok := importAdmin(w, r)
	if !ok {
		return
	}
	content, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || content != "text/csv" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) || r.Header.Get("Content-Encoding") != "" {
		reject(w, 415, "csv_content_type_required")
		return
	}
	if r.URL.RawQuery != "" {
		reject(w, 400, "invalid_input")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, service.MaxImportBytes))
	if err != nil {
		var limitError *http.MaxBytesError
		if errors.As(err, &limitError) {
			reject(w, 413, "csv_too_large")
		} else {
			reject(w, 400, "invalid_csv")
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	report, err := a.service.Import(ctx, actor.FirebaseUID, raw)
	if err != nil {
		if errors.Is(err, domain.ErrInvalid) {
			reject(w, 400, "invalid_csv")
		} else {
			failure(w, err)
		}
		return
	}
	reply(w, 200, report)
}
