package handler

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type QRService interface {
	PNG(context.Context, string, string) ([]byte, error)
}

type QRAPI struct {
	service QRService
	timeout time.Duration
}

func NewQRAPI(service QRService, timeout time.Duration) *QRAPI { return &QRAPI{service, timeout} }

func (a *QRAPI) Own(w http.ResponseWriter, r *http.Request) { a.get(w, r, true) }
func (a *QRAPI) Get(w http.ResponseWriter, r *http.Request) { a.get(w, r, false) }

func (a *QRAPI) get(w http.ResponseWriter, r *http.Request, own bool) {
	actor, ok := authentication.AccountFromContext(r.Context())
	if !ok {
		reject(w, 401, "unauthorized")
		return
	}
	if !actor.IsActive || (own && actor.Role != domain.RoleStudent) ||
		(!own && actor.Role != domain.RoleAdmin && actor.Role != domain.RoleWarden) {
		reject(w, 403, "forbidden")
		return
	}
	id := ""
	if !own {
		id = r.PathValue("studentID")
		if !validation.UUID(id) {
			failure(w, sdomain.ErrInvalid)
			return
		}
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(body) != 0 || r.URL.RawQuery != "" {
		failure(w, sdomain.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	image, err := a.service.PNG(ctx, actor.FirebaseUID, id)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `inline; filename="student-qr.png"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image)
}
