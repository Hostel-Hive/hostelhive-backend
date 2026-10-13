package handler

import (
	"context"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
)

type ImageService interface {
	Upload(context.Context, string, string, string, string, []byte) (d.ImageResult, error)
	Get(context.Context, string, string, string) ([]byte, d.Image, error)
	List(context.Context, string, string) (d.ImageList, error)
	Remove(context.Context, string, string, string) (d.ImageRemoval, error)
}
type ImageAPI struct {
	service ImageService
	timeout time.Duration
}

func NewImageAPI(s ImageService, t time.Duration) *ImageAPI { return &ImageAPI{s, t} }
func imageFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, d.ErrUnavailable) {
		response.Error(w, 503, "complaint_images_unavailable")
		return
	}
	failure(w, err)
}
func imageRequest(w http.ResponseWriter, r *http.Request, write, body bool) (string, bool) {
	uid, ok := authorized(w, r, write, false)
	if !ok {
		return "", false
	}
	if r.URL.RawQuery != "" {
		failure(w, d.ErrInvalid)
		return "", false
	}
	if !body && !empty(w, r) {
		return "", false
	}
	return uid, true
}
func (a *ImageAPI) Upload(w http.ResponseWriter, r *http.Request) {
	uid, ok := imageRequest(w, r, true, true)
	if !ok {
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (media != "image/jpeg" && media != "image/png") {
		response.Error(w, 415, "unsupported_media_type")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, d.MaxComplaintImageBytes))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			response.Error(w, 413, "payload_too_large")
		} else {
			failure(w, d.ErrInvalid)
		}
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, err := a.service.Upload(ctx, uid, r.PathValue("complaintID"), r.PathValue("imageID"), media, raw)
	if err != nil {
		imageFailure(w, err)
		return
	}
	status := 201
	if v.Replayed {
		status = 200
	}
	reply(w, status, v)
}
func (a *ImageAPI) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := imageRequest(w, r, false, false)
	if !ok {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	raw, m, err := a.service.Get(ctx, uid, r.PathValue("complaintID"), r.PathValue("imageID"))
	if err != nil {
		imageFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}
func (a *ImageAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := imageRequest(w, r, false, false)
	if !ok {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, err := a.service.List(ctx, uid, r.PathValue("complaintID"))
	if err != nil {
		imageFailure(w, err)
		return
	}
	reply(w, 200, v)
}
func (a *ImageAPI) Remove(w http.ResponseWriter, r *http.Request) {
	uid, ok := imageRequest(w, r, true, false)
	if !ok {
		return
	}
	ctx, c := context.WithTimeout(r.Context(), a.timeout)
	defer c()
	v, err := a.service.Remove(ctx, uid, r.PathValue("complaintID"), r.PathValue("imageID"))
	if err != nil {
		imageFailure(w, err)
		return
	}
	reply(w, 200, v)
}
