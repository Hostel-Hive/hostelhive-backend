package handler

import (
	"context"
	"io"
	"mime"
	"net/http"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type ImageService interface {
	Upload(context.Context, string, string, string, []byte) (d.Profile, error)
	Get(context.Context, string) ([]byte, d.Image, error)
	Remove(context.Context, string, string) error
}
type ImageAPI struct {
	service ImageService
	timeout time.Duration
}

func NewImageAPI(s ImageService, timeout time.Duration) *ImageAPI { return &ImageAPI{s, timeout} }
func (a *ImageAPI) valid(w http.ResponseWriter, r *http.Request) bool {
	if !validation.UUID(r.PathValue("studentID")) {
		reject(w, 400, "invalid_input")
		return false
	}
	if a.service == nil {
		reject(w, 503, "student_images_unavailable")
		return false
	}
	return true
}
func (a *ImageAPI) Upload(w http.ResponseWriter, r *http.Request) {
	actor, ok := staff(w, r)
	if !ok || !a.valid(w, r) {
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (contentType != "image/jpeg" && contentType != "image/png") {
		reject(w, 415, "unsupported_image_type")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, d.MaxImageBytes))
	if err != nil {
		reject(w, 413, "image_too_large")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	profile, err := a.service.Upload(ctx, actor.FirebaseUID, r.PathValue("studentID"), contentType, raw)
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, profile)
}
func (a *ImageAPI) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := staff(w, r); !ok || !a.valid(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	data, meta, err := a.service.Get(ctx, r.PathValue("studentID"))
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
func (a *ImageAPI) Remove(w http.ResponseWriter, r *http.Request) {
	actor, ok := staff(w, r)
	if !ok || !a.valid(w, r) {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(raw) != 0 {
		reject(w, 400, "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	if err = a.service.Remove(ctx, actor.FirebaseUID, r.PathValue("studentID")); err != nil {
		failure(w, err)
		return
	}
	reply(w, 204, nil)
}
