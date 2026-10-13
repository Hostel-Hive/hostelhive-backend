package handler_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type qrService struct {
	err       error
	calls     int
	actor, id string
}

func (s *qrService) PNG(ctx context.Context, actor, id string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing QR deadline")
	}
	s.calls++
	s.actor, s.id = actor, id
	return []byte("png"), s.err
}

func TestQRHTTPPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, role, path, body string
		token, inactive        bool
		err                    error
		status, calls          int
	}{
		{"own", "student", "/api/v1/me/qr", "", true, false, nil, 200, 1},
		{"admin", "admin", "/api/v1/students/" + testID + "/qr", "", true, false, nil, 200, 1},
		{"warden", "warden", "/api/v1/students/" + testID + "/qr", "", true, false, nil, 200, 1},
		{"anonymous", "student", "/api/v1/me/qr", "", false, false, nil, 401, 0},
		{"inactive", "student", "/api/v1/me/qr", "", true, true, nil, 403, 0},
		{"other student", "student", "/api/v1/students/" + testID + "/qr", "", true, false, nil, 403, 0},
		{"staff self", "admin", "/api/v1/me/qr", "", true, false, nil, 403, 0},
		{"security", "security_staff", "/api/v1/students/" + testID + "/qr", "", true, false, nil, 403, 0},
		{"sub warden", "sub_warden", "/api/v1/students/" + testID + "/qr", "", true, false, nil, 403, 0},
		{"bad UUID", "admin", "/api/v1/students/bad/qr", "", true, false, nil, 400, 0},
		{"query injection", "student", "/api/v1/me/qr?student_id=" + testID, "", true, false, nil, 400, 0},
		{"body", "student", "/api/v1/me/qr", "{}", true, false, nil, 400, 0},
		{"large body", "student", "/api/v1/me/qr", strings.Repeat("a", 10), true, false, nil, 400, 0},
		{"missing or ineligible", "admin", "/api/v1/students/" + testID + "/qr", "", true, false, d.ErrNotFound, 404, 1},
		{"fresh role", "student", "/api/v1/me/qr", "", true, false, d.ErrForbidden, 403, 1},
		{"database", "student", "/api/v1/me/qr", "", true, false, errors.New("private SQL token"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &qrService{err: tc.err}
			mux := http.NewServeMux()
			student.RegisterQR(h.NewQRAPI(fake, time.Second), mux, m.Middleware(verifier{}, accounts{tc.role, !tc.inactive}, time.Second))
			w := request(mux, "GET", tc.path, tc.body, tc.token)
			if w.Code != tc.status || fake.calls != tc.calls {
				t.Fatalf("status %d calls %d body %s", w.Code, fake.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("QR response cacheable")
			}
			if strings.Contains(w.Body.String(), "private SQL") {
				t.Fatal("internal error leaked")
			}
			if tc.status == 200 {
				if fake.actor != "verified-admin" || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal("bad actor or PNG headers")
				}
				if tc.role == "student" && fake.id != "" {
					t.Fatal("self lookup allowed client identity")
				}
			}
		})
	}
}
