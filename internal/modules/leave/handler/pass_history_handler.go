package handler

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/service"
)

type PassHistoryAPI struct {
	service service.PassHistoryReader
	timeout time.Duration
}

func NewPassHistory(s service.PassHistoryReader, timeout time.Duration) *PassHistoryAPI {
	return &PassHistoryAPI{s, timeout}
}

func (a *PassHistoryAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := authorized(w, r, true, false)
	if !ok || !empty(w, r) {
		return
	}
	f, err := passQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	page, err := a.service.ListPasses(ctx, uid, f)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, http.StatusOK, page)
}

func passQuery(raw string) (domain.PassFilter, error) {
	f := domain.PassFilter{Limit: 20}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, domain.ErrInvalid
	}
	for key, values := range q {
		if len(values) != 1 || values[0] == "" {
			return f, domain.ErrInvalid
		}
		switch key {
		case "type":
			f.Type = values[0]
		case "status":
			f.Status = values[0]
		case "limit", "offset":
			n, err := strconv.Atoi(values[0])
			if err != nil {
				return f, domain.ErrInvalid
			}
			if key == "limit" {
				f.Limit = n
			} else {
				f.Offset = n
			}
		default:
			return f, domain.ErrInvalid
		}
	}
	return f, nil
}
