package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"strings"
	"testing"
	"time"
)

type ownProfileFake struct {
	calls int
	actor string
	err   error
	image bool
}

func (f *ownProfileFake) Get(ctx context.Context, actor string) (d.OwnProfile, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing timeout")
	}
	f.calls++
	f.actor = actor
	p := d.OwnProfile{StudentID: testID, Guardians: []d.Guardian{}, Presence: d.OwnPresence{Status: "UNKNOWN"}}
	if f.image {
		url := "/api/v1/me/student-profile/image"
		p.ProfileImageURL = &url
	}
	return p, f.err
}
func TestOwnProfileHTTPPolicy(t *testing.T) {
	for _, image := range []bool{false, true} {
		path := "/api/v1/me/student-profile"
		if image {
			path += "/image"
		}
		for _, tc := range []struct {
			name, role, query, body string
			token, active           bool
			err                     error
			status, calls           int
		}{
			{"student", "student", "", "", true, true, nil, 200, 1},
			{"anonymous", "student", "", "", false, true, nil, 401, 0},
			{"inactive", "student", "", "", true, false, nil, 403, 0},
			{"admin", "admin", "", "", true, true, nil, 403, 0},
			{"warden", "warden", "", "", true, true, nil, 403, 0},
			{"sub warden", "sub_warden", "", "", true, true, nil, 403, 0},
			{"security", "security_staff", "", "", true, true, nil, 403, 0},
			{"owner query", "student", "?student_id=" + testID, "", true, true, nil, 400, 0},
			{"body", "student", "", "{}", true, true, nil, 400, 0},
			{"large body", "student", "", strings.Repeat("x", 100), true, true, nil, 400, 0},
			{"missing profile", "student", "", "", true, true, d.ErrNotFound, 404, 1},
			{"fresh role", "student", "", "", true, true, d.ErrForbidden, 403, 1},
			{"storage error", "student", "", "", true, true, errors.New("private SQL"), 503, 1},
		} {
			t.Run(path+tc.name, func(t *testing.T) {
				f := &ownProfileFake{err: tc.err, image: image}
				images := &imageFake{}
				mux := http.NewServeMux()
				student.RegisterOwnProfile(h.NewOwnProfileAPI(f, images, time.Second), mux, m.Middleware(verifier{}, accounts{tc.role, tc.active}, time.Second))
				w := request(mux, "GET", path+tc.query, tc.body, tc.token)
				if w.Code != tc.status || f.calls != tc.calls {
					t.Fatalf("status %d calls %d: %s", w.Code, f.calls, w.Body.String())
				}
				if w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "private SQL") {
					t.Fatal("unsafe response")
				}
				if tc.status == 200 {
					if f.actor != "verified-admin" {
						t.Fatal("wrong authenticated identity")
					}
					if image {
						if images.calls != 1 || w.Body.String() != "image" || w.Header().Get("Content-Type") != "image/png" {
							t.Fatal("bad image")
						}
					} else {
						var decoded map[string]any
						if json.Unmarshal(w.Body.Bytes(), &decoded) != nil || decoded["accommodation"] != nil || decoded["profile_image_url"] != nil || len(decoded["guardians"].([]any)) != 0 {
							t.Fatal("bad optional fields")
						}
						for _, key := range []string{"firebase_uid", "user_id", "account_active", "profile_image_key"} {
							if _, ok := decoded[key]; ok {
								t.Fatal("internal field", key)
							}
						}
					}
				} else if images.calls != 0 {
					t.Fatal("unauthorized image fetch")
				}
			})
		}
	}
}
func TestOwnProfileImageAvailability(t *testing.T) {
	for _, tc := range []struct {
		image, configured bool
		err               error
		status            int
	}{
		{false, false, nil, 404}, {true, false, nil, 503}, {true, true, d.ErrNotFound, 404}, {true, true, d.ErrUnavailable, 503},
	} {
		f := &ownProfileFake{image: tc.image}
		var images h.ImageService
		if tc.configured {
			images = &imageFake{err: tc.err}
		}
		mux := http.NewServeMux()
		student.RegisterOwnProfile(h.NewOwnProfileAPI(f, images, time.Second), mux, m.Middleware(verifier{}, accounts{"student", true}, time.Second))
		w := request(mux, "GET", "/api/v1/me/student-profile/image", "", true)
		if w.Code != tc.status {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}
