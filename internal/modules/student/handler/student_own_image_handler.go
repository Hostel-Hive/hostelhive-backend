package handler

import (
	"context"
	"errors"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	auth "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"io"
	"mime"
	"net/http"
	"time"
)

type OwnImageService interface {
	Upload(context.Context, string, string, []byte) (d.OwnImageResult, error)
	Remove(context.Context, string) error
}
type OwnImageAPI struct {
	service OwnImageService
	timeout time.Duration
}

func NewOwnImageAPI(s OwnImageService, timeout time.Duration) *OwnImageAPI {
	return &OwnImageAPI{s, timeout}
}
func (a *OwnImageAPI) Upload(w http.ResponseWriter, r *http.Request) { a.write(w, r, true) }
func (a *OwnImageAPI) Remove(w http.ResponseWriter, r *http.Request) { a.write(w, r, false) }
func (a *OwnImageAPI) write(w http.ResponseWriter, r *http.Request, upload bool) {
	actor, ok := auth.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return
	}
	if !actor.IsActive || actor.Role != identity.RoleStudent {
		reject(w, 403, "forbidden")
		return
	}
	if r.URL.RawQuery != "" {
		reject(w, 400, "invalid_input")
		return
	}
	var raw []byte
	var contentType string
	if upload {
		var err error
		contentType, _, err = mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || (contentType != "image/jpeg" && contentType != "image/png") {
			reject(w, 415, "unsupported_image_type")
			return
		}
		raw, err = io.ReadAll(http.MaxBytesReader(w, r.Body, d.MaxImageBytes))
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				reject(w, 413, "image_too_large")
			} else {
				reject(w, 400, "invalid_input")
			}
			return
		}
	} else {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
		if err != nil || len(body) != 0 {
			reject(w, 400, "invalid_input")
			return
		}
	}
	if a.service == nil {
		reject(w, 503, "student_images_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	if upload {
		result, err := a.service.Upload(ctx, actor.FirebaseUID, contentType, raw)
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, result)
	} else {
		if err := a.service.Remove(ctx, actor.FirebaseUID); err != nil {
			failure(w, err)
			return
		}
		reply(w, 204, nil)
	}
}
