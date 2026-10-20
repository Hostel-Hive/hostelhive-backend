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

type AllowanceAPI struct {
	service service.AllowanceReader
	timeout time.Duration
}

func NewAllowance(s service.AllowanceReader, timeout time.Duration) *AllowanceAPI {
	return &AllowanceAPI{s, timeout}
}

func (a *AllowanceAPI) Get(w http.ResponseWriter, r *http.Request) {
	actor, ok := authorized(w, r, true, false)
	if !ok {
		return
	}
	if !empty(w, r) {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, domain.ErrInvalid)
		return
	}
	year := 0
	for name, values := range query {
		if name != "year" || len(values) != 1 || len(values[0]) != 4 {
			failure(w, domain.ErrInvalid)
			return
		}
		for _, digit := range values[0] {
			if digit < '0' || digit > '9' {
				failure(w, domain.ErrInvalid)
				return
			}
		}
		year, err = strconv.Atoi(values[0])
		if err != nil || year < 1 || year > 9998 {
			failure(w, domain.ErrInvalid)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	summary, err := a.service.GetAllowance(ctx, actor, year)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, http.StatusOK, summary)
}
