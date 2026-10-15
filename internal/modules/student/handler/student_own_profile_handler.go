package handler

import (
	"context"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"io"
	"net/http"
	"time"
)

type OwnProfileService interface {
	Get(context.Context, string) (d.OwnProfile, error)
}
type OwnProfileAPI struct {
	service OwnProfileService
	images  ImageService
	timeout time.Duration
}

func NewOwnProfileAPI(service OwnProfileService, images ImageService, timeout time.Duration) *OwnProfileAPI {
	return &OwnProfileAPI{service: service, images: images, timeout: timeout}
}
func (a *OwnProfileAPI) Get(w http.ResponseWriter, r *http.Request)   { a.read(w, r, false) }
func (a *OwnProfileAPI) Image(w http.ResponseWriter, r *http.Request) { a.read(w, r, true) }
func (a *OwnProfileAPI) read(w http.ResponseWriter, r *http.Request, image bool) {
	actor, ok := authentication.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return
	}
	if !actor.IsActive || actor.Role != identity.RoleStudent {
		reject(w, 403, "forbidden")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(body) != 0 || r.URL.RawQuery != "" {
		failure(w, d.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	profile, err := a.service.Get(ctx, actor.FirebaseUID)
	if err != nil {
		failure(w, err)
		return
	}
	if !image {
		reply(w, 200, profile)
		return
	}
	if profile.ProfileImageURL == nil {
		failure(w, d.ErrNotFound)
		return
	}
	if a.images == nil {
		reject(w, 503, "student_images_unavailable")
		return
	}
	data, meta, err := a.images.Get(ctx, profile.StudentID)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
