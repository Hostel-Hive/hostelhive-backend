package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		status, code := 503, "service_unavailable"
		if errors.Is(err, domain.ErrInvalid) {
			status, code = 400, "invalid_report_query"
		}
		if errors.Is(err, domain.ErrForbidden) {
			status, code = 403, "forbidden"
		}
		response.Error(w, status, code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
