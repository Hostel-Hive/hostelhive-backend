package handler

import (
	"context"
	"encoding/json"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
	"io"
	"mime"
	"net/http"
	"time"
)

type PushAPI struct {
	service service.PushDevices
	timeout time.Duration
}

func NewPush(s service.PushDevices, t time.Duration) *PushAPI { return &PushAPI{s, t} }
func pushAuthorized(w http.ResponseWriter, r *http.Request) (string, bool) {
	a, ok := middleware.AccountFromContext(r.Context())
	if !ok {
		response.Error(w, 401, "unauthorized")
		return "", false
	}
	if !a.IsActive || (a.Role != "admin" && a.Role != "warden" && a.Role != "sub_warden" && a.Role != "student" && a.Role != "security_staff") {
		response.Error(w, 403, "forbidden")
		return "", false
	}
	if r.URL.RawQuery != "" {
		reply(w, nil, domain.ErrInvalid)
		return "", false
	}
	return a.FirebaseUID, true
}
func readPushRegistration(w http.ResponseWriter, r *http.Request) (dto.PushRegistration, bool) {
	var v dto.PushRegistration
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type")
		return v, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		reply(w, nil, domain.ErrInvalid)
		return v, false
	}
	seen := map[string]bool{}
	for decoder.More() {
		name, err := decoder.Token()
		key, ok := name.(string)
		if err != nil || !ok || seen[key] {
			reply(w, nil, domain.ErrInvalid)
			return v, false
		}
		seen[key] = true
		switch key {
		case "platform":
			err = decoder.Decode(&v.Platform)
		case "token":
			err = decoder.Decode(&v.Token)
		default:
			err = domain.ErrInvalid
		}
		if err != nil {
			reply(w, nil, domain.ErrInvalid)
			return v, false
		}
	}
	last, err := decoder.Token()
	var extra any
	if err != nil || last != json.Delim('}') || decoder.Decode(&extra) != io.EOF {
		reply(w, nil, domain.ErrInvalid)
		return v, false
	}
	return v, true
}
func (a *PushAPI) Register(w http.ResponseWriter, r *http.Request) {
	uid, ok := pushAuthorized(w, r)
	if !ok {
		return
	}
	v, ok := readPushRegistration(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	device, err := a.service.RegisterPushDevice(ctx, uid, r.PathValue("deviceID"), v)
	reply(w, device, err)
}
func (a *PushAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := pushAuthorized(w, r)
	if !ok || !empty(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	devices, err := a.service.ListPushDevices(ctx, uid)
	reply(w, devices, err)
}
func (a *PushAPI) Remove(w http.ResponseWriter, r *http.Request) {
	uid, ok := pushAuthorized(w, r)
	if !ok || !empty(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	if err := a.service.RemovePushDevice(ctx, uid, r.PathValue("deviceID")); err != nil {
		reply(w, nil, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
