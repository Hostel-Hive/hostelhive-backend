package student

import (
	"net/http"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

func Register(a *handler.API, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(authentication.RequireRoles(domain.RoleAdmin, domain.RoleWarden)(h))
	}
	mux.Handle("GET /api/v1/students", protect(a.List))
	mux.Handle("GET /api/v1/students/{studentID}", protect(a.Get))
	mux.Handle("POST /api/v1/students", protect(a.Create))
	mux.Handle("PUT /api/v1/students/{studentID}", protect(a.Update))
	mux.Handle("DELETE /api/v1/students/{studentID}", protect(a.Delete))
}

func RegisterImages(a *handler.ImageAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(authentication.RequireRoles(domain.RoleAdmin, domain.RoleWarden)(h))
	}
	mux.Handle("PUT /api/v1/students/{studentID}/image", protect(a.Upload))
	mux.Handle("GET /api/v1/students/{studentID}/image", protect(a.Get))
	mux.Handle("DELETE /api/v1/students/{studentID}/image", protect(a.Remove))
}

// Import remains Admin-only even though ordinary student CRUD allows Wardens.
func RegisterImport(a *handler.ImportAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(authentication.RequireRoles(domain.RoleAdmin)(h))
	}
	mux.Handle("POST /api/v1/students/import", protect(a.Import))
	mux.Handle("GET /api/v1/students/import/template", protect(a.Template))
}

func RegisterQR(a *handler.QRAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/me/qr", authenticate(authentication.RequireRoles(domain.RoleStudent)(http.HandlerFunc(a.Own))))
	mux.Handle("GET /api/v1/students/{studentID}/qr", authenticate(authentication.RequireRoles(domain.RoleAdmin, domain.RoleWarden)(http.HandlerFunc(a.Get))))
}

// RegisterOwnProfile derives the target exclusively from the authenticated account.
func RegisterOwnProfile(a *handler.OwnProfileAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(authentication.RequireRoles(domain.RoleStudent)(h))
	}
	mux.Handle("GET /api/v1/me/student-profile", protect(a.Get))
	mux.Handle("GET /api/v1/me/student-profile/image", protect(a.Image))
}

func RegisterOwnProfileEdit(a *handler.OwnProfileEditAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("PATCH /api/v1/me/student-profile", authenticate(authentication.RequireRoles(domain.RoleStudent)(http.HandlerFunc(a.Patch))))
}

func RegisterOwnImages(a *handler.OwnImageAPI, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(h http.HandlerFunc) http.Handler {
		return authenticate(authentication.RequireRoles(domain.RoleStudent)(h))
	}
	mux.Handle("PUT /api/v1/me/student-profile/image", protect(a.Upload))
	mux.Handle("DELETE /api/v1/me/student-profile/image", protect(a.Remove))
}
