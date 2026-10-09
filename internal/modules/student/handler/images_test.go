package handler_test

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	auth "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type imageFake struct {
	calls int
	err   error
}

func (f *imageFake) Upload(ctx context.Context, actor, id, mime string, raw []byte) (d.Profile, error) {
	f.calls++
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	return d.Profile{StudentID: id}, f.err
}
func (f *imageFake) Get(context.Context, string) ([]byte, d.Image, error) {
	f.calls++
	return []byte("image"), d.Image{ContentType: "image/png"}, f.err
}
func (f *imageFake) Remove(context.Context, string, string) error { f.calls++; return f.err }
func imageRouter(f h.ImageService, role string, active bool) http.Handler {
	mux := http.NewServeMux()
	student.RegisterImages(h.NewImageAPI(f, time.Second), mux, auth.Middleware(verifier{}, accounts{role, active}, time.Second))
	return mux
}
func imageRequest(handler http.Handler, method, path, mime, body string, token bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", mime)
	if token {
		r.Header.Set("Authorization", "Bearer token")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func TestImageRoutesAndValidation(t *testing.T) {
	path := "/api/v1/students/" + testID + "/image"
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		for _, method := range []string{"PUT", "GET", "DELETE"} {
			f := &imageFake{}
			body := ""
			if method == "PUT" {
				body = "image"
			}
			w := imageRequest(imageRouter(f, role, true), method, path, "image/png", body, true)
			want := 403
			if role == "admin" || role == "warden" {
				want = 200
				if method == "DELETE" {
					want = 204
				}
			}
			if w.Code != want {
				t.Fatalf("%s %s %d", role, method, w.Code)
			}
			if want == 403 && f.calls != 0 {
				t.Fatal("unauthorized storage call")
			}
			if want == 200 && method == "GET" && (w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff") {
				t.Fatal("unsafe download headers")
			}
		}
	}
	for _, tc := range []struct {
		method, path, mime, body string
		active, token            bool
		want                     int
	}{{"PUT", path, "image/png", "x", true, false, 401}, {"GET", path, "", "", false, true, 403}, {"PUT", path, "image/svg+xml", "x", true, true, 415}, {"PUT", path, "image/png", strings.Repeat("x", d.MaxImageBytes+1), true, true, 413}, {"GET", "/api/v1/students/bad/image", "", "", true, true, 400}, {"DELETE", path, "", "{}", true, true, 400}} {
		f := &imageFake{}
		w := imageRequest(imageRouter(f, "admin", tc.active), tc.method, tc.path, tc.mime, tc.body, tc.token)
		if w.Code != tc.want || f.calls != 0 {
			t.Fatalf("%d calls=%d", w.Code, f.calls)
		}
	}
	if w := imageRequest(imageRouter(nil, "admin", true), "GET", path, "", "", true); w.Code != 503 {
		t.Fatal("disabled storage", w.Code)
	}
	for _, tc := range []struct {
		err  error
		want int
	}{{d.ErrNotFound, 404}, {d.ErrUnavailable, 503}, {d.ErrInvalid, 400}, {d.ErrForbidden, 403}} {
		w := imageRequest(imageRouter(&imageFake{err: tc.err}, "admin", true), "PUT", path, "image/png", "x", true)
		if w.Code != tc.want {
			t.Fatal(w.Code)
		}
	}
}
