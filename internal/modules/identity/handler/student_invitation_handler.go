package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type InvitationService interface {
	Register(context.Context, string, string, dto.StudentInvitationInput) (d.StudentInvitation, bool, error)
	Get(context.Context, string, string) (d.StudentInvitation, error)
	List(context.Context, string, int, int) (d.InvitationPage, error)
	Resend(context.Context, string, string, string, string) (d.StudentInvitation, error)
	Complete(context.Context, string, string) (d.SetupResult, error)
	Import(context.Context, string, string, []byte) (service.InvitationImportReport, error)
}
type InvitationAPI struct {
	service InvitationService
	timeout time.Duration
}

func NewInvitationAPI(s InvitationService, t time.Duration) *InvitationAPI {
	return &InvitationAPI{s, t}
}
func invitationReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func invitationFailure(w http.ResponseWriter, e error) {
	status, code := 503, "student_invitations_unavailable"
	switch {
	case errors.Is(e, d.ErrInvitationInvalid):
		status, code = 400, "invalid_input"
	case errors.Is(e, d.ErrInvitationConflict), errors.Is(e, d.ErrInvitationIdentity):
		status, code = 409, "invitation_conflict"
	case errors.Is(e, d.ErrInvitationForbidden):
		status, code = 403, "forbidden"
	case errors.Is(e, d.ErrInvitationNotFound):
		status, code = 404, "not_found"
	case errors.Is(e, d.ErrSetupPending):
		status, code = 409, "setup_not_verified"
	case errors.Is(e, d.ErrInvitationRateLimited):
		status, code = 429, "invitation_resend_too_soon"
		w.Header().Set("Retry-After", "300")
	}
	response.Error(w, status, code)
}
func invitationActor(w http.ResponseWriter, r *http.Request, role string) (string, bool) {
	a, ok := m.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", false
	}
	if !a.IsActive || a.Role != role {
		response.Error(w, 403, "forbidden")
		return "", false
	}
	return a.FirebaseUID, true
}
func invitationDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.URL.RawQuery != "" {
		invitationFailure(w, d.ErrInvitationInvalid)
		return false
	}
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	dec.DisallowUnknownFields()
	if e = dec.Decode(v); e == nil {
		var extra any
		if dec.Decode(&extra) == io.EOF {
			return true
		}
	}
	invitationFailure(w, d.ErrInvitationInvalid)
	return false
}
func invitationEmpty(w http.ResponseWriter, r *http.Request, query bool) bool {
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if e != nil || len(body) != 0 || (!query && r.URL.RawQuery != "") {
		invitationFailure(w, d.ErrInvitationInvalid)
		return false
	}
	return true
}

// Use the actual connection peer, not an untrusted forwarded header.
func invitationIP(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return ""
	}
	return host
}
func (a *InvitationAPI) Register(w http.ResponseWriter, r *http.Request) {
	actor, ok := invitationActor(w, r, d.RoleAdmin)
	if !ok {
		return
	}
	var input dto.StudentInvitationInput
	if !invitationDecode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, replayed, e := a.service.Register(ctx, actor, invitationIP(r), input)
	if e != nil {
		invitationFailure(w, e)
		return
	}
	status := 201
	if replayed {
		status = 200
	}
	invitationReply(w, status, struct {
		Invitation d.StudentInvitation `json:"invitation"`
		Replayed   bool                `json:"replayed"`
	}{v, replayed})
}
func (a *InvitationAPI) Get(w http.ResponseWriter, r *http.Request) {
	actor, ok := invitationActor(w, r, d.RoleAdmin)
	if !ok || !invitationEmpty(w, r, false) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.Get(ctx, actor, r.PathValue("invitationID"))
	if e != nil {
		invitationFailure(w, e)
		return
	}
	invitationReply(w, 200, v)
}
func (a *InvitationAPI) List(w http.ResponseWriter, r *http.Request) {
	actor, ok := invitationActor(w, r, d.RoleAdmin)
	if !ok || !invitationEmpty(w, r, true) {
		return
	}
	limit, offset := 20, 0
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		invitationFailure(w, d.ErrInvitationInvalid)
		return
	}
	for key, v := range values {
		if len(v) != 1 || (key != "limit" && key != "offset") {
			invitationFailure(w, d.ErrInvitationInvalid)
			return
		}
		n, e := strconv.Atoi(v[0])
		if e != nil {
			invitationFailure(w, d.ErrInvitationInvalid)
			return
		}
		if key == "limit" {
			limit = n
		} else {
			offset = n
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.List(ctx, actor, limit, offset)
	if e != nil {
		invitationFailure(w, e)
		return
	}
	invitationReply(w, 200, v)
}
func (a *InvitationAPI) Resend(w http.ResponseWriter, r *http.Request) {
	actor, ok := invitationActor(w, r, d.RoleAdmin)
	if !ok {
		return
	}
	var input dto.InvitationResend
	if !invitationDecode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.Resend(ctx, actor, r.PathValue("invitationID"), input.RequestID, invitationIP(r))
	if e != nil {
		invitationFailure(w, e)
		return
	}
	invitationReply(w, 200, v)
}
func (a *InvitationAPI) Complete(w http.ResponseWriter, r *http.Request) {
	actor, ok := invitationActor(w, r, d.RoleStudent)
	if !ok || !invitationEmpty(w, r, false) {
		return
	}
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 {
		response.Error(w, 401, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.Complete(ctx, actor, fields[1])
	if e != nil {
		invitationFailure(w, e)
		return
	}
	invitationReply(w, 200, v)
}
func (a *InvitationAPI) Template(w http.ResponseWriter, r *http.Request) {
	if _, ok := invitationActor(w, r, d.RoleAdmin); !ok || !invitationEmpty(w, r, false) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="student-invitations.csv"`)
	_, _ = io.WriteString(w, service.InvitationCSVHeader+"\r\n")
}
func (a *InvitationAPI) Import(w http.ResponseWriter, r *http.Request) {
	actor, ok := invitationActor(w, r, d.RoleAdmin)
	if !ok {
		return
	}
	if r.URL.RawQuery != "" {
		invitationFailure(w, d.ErrInvitationInvalid)
		return
	}
	media, params, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "text/csv" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		response.Error(w, 415, "unsupported_media_type")
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, service.MaxInvitationCSVBytes))
	if e != nil {
		response.Error(w, 413, "import_too_large")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	v, e := a.service.Import(ctx, actor, invitationIP(r), raw)
	if e != nil {
		invitationFailure(w, e)
		return
	}
	invitationReply(w, 200, v)
}
