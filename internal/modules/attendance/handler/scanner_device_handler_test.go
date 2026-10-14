package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/handler"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

const id = "11111111-1111-4111-8111-111111111111"

type verifier struct{}

func (verifier) Verify(_ context.Context, token string) (string, error) {
	if token != "firebase-test" {
		return "", errors.New("invalid Firebase token")
	}
	return "verified-admin", nil
}

type accounts struct {
	role   string
	active bool
}

func (a accounts) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return identity.Account{FirebaseUID: "verified-admin", Role: a.role, IsActive: a.active}, nil
}

type fakeService struct {
	err               error
	calls             int
	actor, credential string
}

func (f *fakeService) note(ctx context.Context, actor string) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing timeout")
	}
	f.calls++
	f.actor = actor
}
func (f *fakeService) Register(ctx context.Context, actor string, in dto.RegisterDevice) (dto.IssuedCredential, error) {
	f.note(ctx, actor)
	return dto.IssuedCredential{Device: d.Device{DeviceID: id, Name: in.Name}, Credential: "issued-only"}, f.err
}
func (f *fakeService) List(ctx context.Context, actor string, limit, offset int) (d.DevicePage, error) {
	f.note(ctx, actor)
	return d.DevicePage{Items: []d.Device{}, Limit: limit, Offset: offset}, f.err
}
func (f *fakeService) Rotate(ctx context.Context, actor, id string) (dto.IssuedCredential, error) {
	f.note(ctx, actor)
	return dto.IssuedCredential{Credential: "issued-only"}, f.err
}
func (f *fakeService) Deactivate(ctx context.Context, actor, id string) (d.Device, error) {
	f.note(ctx, actor)
	return d.Device{DeviceID: id}, f.err
}
func (f *fakeService) Authenticate(ctx context.Context, credential string) (d.Device, error) {
	f.note(ctx, "")
	f.credential = credential
	return d.Device{DeviceID: id, Name: "Gate-01", IsActive: true}, f.err
}

func TestScannerRoutesAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body, media, token, role string
		inactive                                     bool
		err                                          error
		status, calls                                int
	}{
		{"register", "POST", "/api/v1/scanner-devices", `{"name":"Gate-01"}`, "application/json", "firebase-test", "admin", false, nil, 201, 1},
		{"list", "GET", "/api/v1/scanner-devices?limit=2&offset=1", "", "", "firebase-test", "admin", false, nil, 200, 1},
		{"rotate", "POST", "/api/v1/scanner-devices/" + id + "/rotate-credential", "", "", "firebase-test", "admin", false, nil, 200, 1},
		{"deactivate", "POST", "/api/v1/scanner-devices/" + id + "/deactivate", "", "", "firebase-test", "admin", false, nil, 200, 1},
		{"check", "GET", "/api/v1/device/check", "", "", "scanner-test", "", false, nil, 200, 1},
		{"anonymous", "GET", "/api/v1/scanner-devices", "", "", "", "admin", false, nil, 401, 0},
		{"inactive admin", "GET", "/api/v1/scanner-devices", "", "", "firebase-test", "admin", true, nil, 403, 0},
		{"warden", "GET", "/api/v1/scanner-devices", "", "", "firebase-test", "warden", false, nil, 403, 0},
		{"student", "POST", "/api/v1/scanner-devices", `{"name":"gate"}`, "application/json", "firebase-test", "student", false, nil, 403, 0},
		{"non JSON", "POST", "/api/v1/scanner-devices", `{"name":"gate"}`, "text/plain", "firebase-test", "admin", false, nil, 415, 0},
		{"unknown actor", "POST", "/api/v1/scanner-devices", `{"name":"gate","actor":"fake"}`, "application/json", "firebase-test", "admin", false, nil, 400, 0},
		{"trailing JSON", "POST", "/api/v1/scanner-devices", `{"name":"gate"}{}`, "application/json", "firebase-test", "admin", false, nil, 400, 0},
		{"malformed", "POST", "/api/v1/scanner-devices", `{`, "application/json", "firebase-test", "admin", false, nil, 400, 0},
		{"UTF8", "POST", "/api/v1/scanner-devices", string([]byte{0xff}), "application/json", "firebase-test", "admin", false, nil, 400, 0},
		{"oversize", "POST", "/api/v1/scanner-devices", strings.Repeat("a", 2049), "application/json", "firebase-test", "admin", false, nil, 413, 0},
		{"query create", "POST", "/api/v1/scanner-devices?actor=fake", `{"name":"gate"}`, "application/json", "firebase-test", "admin", false, nil, 400, 0},
		{"unknown filter", "GET", "/api/v1/scanner-devices?credential=secret", "", "", "firebase-test", "admin", false, nil, 400, 0},
		{"duplicate filter", "GET", "/api/v1/scanner-devices?limit=1&limit=2", "", "", "firebase-test", "admin", false, nil, 400, 0},
		{"bad filter", "GET", "/api/v1/scanner-devices?limit=x", "", "", "firebase-test", "admin", false, nil, 400, 0},
		{"list body", "GET", "/api/v1/scanner-devices", "{}", "", "firebase-test", "admin", false, nil, 400, 0},
		{"rotation body", "POST", "/api/v1/scanner-devices/" + id + "/rotate-credential", "{}", "", "firebase-test", "admin", false, nil, 400, 0},
		{"deactivation query", "POST", "/api/v1/scanner-devices/" + id + "/deactivate?active=true", "", "", "firebase-test", "admin", false, nil, 400, 0},
		{"check body", "GET", "/api/v1/device/check", "{}", "", "scanner-test", "", false, nil, 400, 1},
		{"check query", "GET", "/api/v1/device/check?credential=secret", "", "", "scanner-test", "", false, nil, 400, 1},
		{"missing device credential", "GET", "/api/v1/device/check", "", "", "", "", false, nil, 401, 0},
		{"wrong scheme", "GET", "/api/v1/device/check", "", "", "Basic anything", "", false, nil, 401, 0},
		{"authentication error", "GET", "/api/v1/device/check", "", "", "scanner-test", "", false, d.ErrUnauthorized, 401, 1},
		{"fresh admin error", "GET", "/api/v1/scanner-devices", "", "", "firebase-test", "admin", false, d.ErrForbidden, 403, 1},
		{"invalid input", "POST", "/api/v1/scanner-devices", `{"name":""}`, "application/json", "firebase-test", "admin", false, d.ErrInvalid, 400, 1},
		{"missing device", "POST", "/api/v1/scanner-devices/" + id + "/deactivate", "", "", "firebase-test", "admin", false, d.ErrNotFound, 404, 1},
		{"duplicate", "POST", "/api/v1/scanner-devices", `{"name":"gate"}`, "application/json", "firebase-test", "admin", false, d.ErrConflict, 409, 1},
		{"DB error", "POST", "/api/v1/scanner-devices", `{"name":"gate"}`, "application/json", "firebase-test", "admin", false, errors.New("private hash SQL"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeService{err: tc.err}
			mux := http.NewServeMux()
			attendance.Register(h.New(fake, time.Second, time.Second), mux, m.Middleware(verifier{}, accounts{tc.role, !tc.inactive}, time.Second))
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.media != "" {
				r.Header.Set("Content-Type", tc.media)
			}
			if tc.token != "" {
				if tc.name == "wrong scheme" {
					r.Header.Set("Authorization", tc.token)
				} else {
					r.Header.Set("Authorization", "Bearer "+tc.token)
				}
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status || fake.calls != tc.calls {
				t.Fatalf("HTTP %d calls %d: %s", w.Code, fake.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "private hash") {
				t.Fatal("secret/cache leak")
			}
			if tc.status >= 400 && strings.Contains(w.Body.String(), "issued-only") {
				t.Fatal("failed issuance leaked secret")
			}
			if tc.status < 300 && tc.path == "/api/v1/device/check" && (strings.Contains(w.Body.String(), "credential") || !strings.Contains(w.Body.String(), "authenticated")) {
				t.Fatal("bad check response")
			}
			if tc.calls > 0 && !strings.HasPrefix(tc.path, "/api/v1/device/") && fake.actor != "verified-admin" {
				t.Fatal("unverified actor")
			}
		})
	}
}

func TestDeviceDuplicateHeadersAndNoContext(t *testing.T) {
	fake := &fakeService{}
	a := h.New(fake, time.Second, time.Second)
	r := httptest.NewRequest("GET", "/api/v1/device/check", nil)
	r.Header.Add("Authorization", "Bearer scanner-test")
	r.Header.Add("Authorization", "Bearer scanner-test")
	w := httptest.NewRecorder()
	a.DeviceMiddleware(http.HandlerFunc(a.Check)).ServeHTTP(w, r)
	if w.Code != 401 || fake.calls != 0 {
		t.Fatal("duplicate authorization accepted")
	}
	w = httptest.NewRecorder()
	a.Check(w, httptest.NewRequest("GET", "/api/v1/device/check", nil))
	if w.Code != 401 {
		t.Fatal("missing device context accepted")
	}
	w = httptest.NewRecorder()
	a.List(w, httptest.NewRequest("GET", "/api/v1/scanner-devices", nil))
	if w.Code != 401 {
		t.Fatal("missing admin context accepted")
	}
}
