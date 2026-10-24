package handler_test

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type ownImageFake struct {
	calls int
	actor string
	err   error
}

func (f *ownImageFake) Upload(ctx context.Context, actor, mime string, raw []byte) (d.OwnImageResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	f.calls++
	f.actor = actor
	return d.OwnImageResult{StudentID: testID, ProfileImageURL: "/api/v1/me/student-profile/image"}, f.err
}
func (f *ownImageFake) Remove(ctx context.Context, actor string) error {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	f.calls++
	f.actor = actor
	return f.err
}
func TestOwnImageHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, method, role, query, body, mime string
		token, active, disabled               bool
		err                                   error
		status, calls                         int
	}{
		{name: "upload", method: "PUT", role: "student", body: "bytes", mime: "image/png", token: true, active: true, status: 200, calls: 1},
		{name: "jpeg", method: "PUT", role: "student", body: "bytes", mime: "image/jpeg", token: true, active: true, status: 200, calls: 1},
		{name: "remove", method: "DELETE", role: "student", token: true, active: true, status: 204, calls: 1},
		{name: "anonymous", method: "PUT", role: "student", mime: "image/png", active: true, status: 401},
		{name: "inactive", method: "DELETE", role: "student", token: true, status: 403},
		{name: "admin", method: "PUT", role: "admin", token: true, active: true, status: 403},
		{name: "warden", method: "DELETE", role: "warden", token: true, active: true, status: 403},
		{name: "security", method: "PUT", role: "security_staff", token: true, active: true, status: 403},
		{name: "subwarden", method: "DELETE", role: "sub_warden", token: true, active: true, status: 403},
		{name: "upload selector", method: "PUT", role: "student", query: "?student_id=" + testID, mime: "image/png", token: true, active: true, status: 400},
		{name: "remove selector", method: "DELETE", role: "student", query: "?user_id=" + testID, token: true, active: true, status: 400},
		{name: "remove body", method: "DELETE", role: "student", body: "{}", token: true, active: true, status: 400},
		{name: "json", method: "PUT", role: "student", body: "{}", mime: "application/json", token: true, active: true, status: 415},
		{name: "multipart", method: "PUT", role: "student", mime: "multipart/form-data; boundary=abc", token: true, active: true, status: 415},
		{name: "svg", method: "PUT", role: "student", mime: "image/svg+xml", token: true, active: true, status: 415},
		{name: "oversized", method: "PUT", role: "student", body: strings.Repeat("x", d.MaxImageBytes+1), mime: "image/png", token: true, active: true, status: 413},
		{name: "disabled upload", method: "PUT", role: "student", body: "bytes", mime: "image/png", token: true, active: true, disabled: true, status: 503},
		{name: "disabled removal", method: "DELETE", role: "student", token: true, active: true, disabled: true, status: 503},
		{name: "bad image", method: "PUT", role: "student", mime: "image/png", token: true, active: true, err: d.ErrInvalid, status: 400, calls: 1},
		{name: "missing", method: "DELETE", role: "student", token: true, active: true, err: d.ErrNotFound, status: 404, calls: 1},
		{name: "fresh authority", method: "PUT", role: "student", mime: "image/png", token: true, active: true, err: d.ErrForbidden, status: 403, calls: 1},
		{name: "storage secret", method: "PUT", role: "student", mime: "image/png", token: true, active: true, err: errors.New("private secret"), status: 503, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &ownImageFake{err: tc.err}
			var service h.OwnImageService = f
			if tc.disabled {
				service = nil
			}
			mux := http.NewServeMux()
			student.RegisterOwnImages(h.NewOwnImageAPI(service, time.Second), mux, m.Middleware(verifier{}, accounts{tc.role, tc.active}, time.Second))
			req := httptest.NewRequest(tc.method, "/api/v1/me/student-profile/image"+tc.query, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.mime)
			if tc.token {
				req.Header.Set("Authorization", "Bearer test")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != tc.status || f.calls != tc.calls || strings.Contains(w.Body.String(), "private secret") {
				t.Fatal(w.Code, w.Body, f.calls)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Header())
			}
			if f.calls > 0 && f.actor == "" {
				t.Fatal("missing authenticated actor")
			}
			if tc.status == 204 && w.Body.Len() != 0 {
				t.Fatal("DELETE response body")
			}
		})
	}
}
