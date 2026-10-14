package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"unicode/utf8"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/staff/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

func (a *API) PutSelf(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return
	}
	if !actor.IsActive || (actor.Role != identity.RoleAdmin && actor.Role != identity.RoleWarden && actor.Role != identity.RoleSubWarden && actor.Role != identity.RoleSecurityStaff) {
		response.Error(w, 403, "forbidden")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil || !utf8.Valid(raw) {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	var d dto.SelfDetails
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&d); err != nil {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		reply(w, nil, domain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.PutSelf(ctx, actor.FirebaseUID, d)
	reply(w, v, e)
}
