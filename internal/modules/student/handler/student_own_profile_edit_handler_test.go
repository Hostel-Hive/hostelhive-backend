package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type ownEditFake struct {
	calls int
	actor string
	err   error
}

func (f *ownEditFake) PatchOwnProfile(ctx context.Context, actor string, _ dto.OwnProfilePatch) (d.OwnProfile, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	f.calls++
	f.actor = actor
	return d.OwnProfile{StudentID: testID, Guardians: []d.Guardian{}, Presence: d.OwnPresence{Status: "UNKNOWN"}}, f.err
}

func TestOwnProfilePatchHTTP(t *testing.T) {
	valid := `{"contact_phone":"0771234567"}`
	guardian := `{"name":"Guardian","relationship":"Mother","contact_phone":"0712345678"}`
	for _, tc := range []struct {
		name, role, query, body, content string
		token, active                    bool
		err                              error
		status, calls                    int
	}{
		{"student", "student", "", valid, "application/json", true, true, nil, 200, 1},
		{"charset", "student", "", valid, "application/json; charset=utf-8", true, true, nil, 200, 1},
		{"anonymous", "student", "", valid, "application/json", false, true, nil, 401, 0},
		{"inactive", "student", "", valid, "application/json", true, false, nil, 403, 0},
		{"admin", "admin", "", valid, "application/json", true, true, nil, 403, 0},
		{"warden", "warden", "", valid, "application/json", true, true, nil, 403, 0},
		{"sub warden", "sub_warden", "", valid, "application/json", true, true, nil, 403, 0},
		{"security", "security_staff", "", valid, "application/json", true, true, nil, 403, 0},
		{"selector", "student", "?student_id=" + testID, valid, "application/json", true, true, nil, 400, 0},
		{"content type", "student", "", valid, "text/plain", true, true, nil, 415, 0},
		{"large body", "student", "", strings.Repeat(" ", 16*1024) + valid, "application/json", true, true, nil, 413, 0},
		{"missing", "student", "", valid, "application/json", true, true, d.ErrNotFound, 404, 1},
		{"fresh role", "student", "", valid, "application/json", true, true, d.ErrForbidden, 403, 1},
		{"storage", "student", "", valid, "application/json", true, true, errors.New("private SQL"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &ownEditFake{err: tc.err}
			mux := http.NewServeMux()
			student.RegisterOwnProfileEdit(h.NewOwnProfileEditAPI(s.NewOwnProfileEditor(f), time.Second), mux, m.Middleware(verifier{}, accounts{tc.role, tc.active}, time.Second))
			r := httptest.NewRequest("PATCH", "/api/v1/me/student-profile"+tc.query, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.content)
			if tc.token {
				r.Header.Set("Authorization", "Bearer test")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status || f.calls != tc.calls || strings.Contains(w.Body.String(), "private SQL") {
				t.Fatal(w.Code, w.Body, f.calls)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Header())
			}
			if tc.calls > 0 && f.actor != "verified-admin" {
				t.Fatal("client selected identity")
			}
		})
	}
	for _, body := range []string{
		``, `{}`, `null`, `[]`, valid + ` {}`, `{"contact_phone":null}`, `{"guardians":null}`, `{"guardians":[]}`,
		`{"contact_phone":null,"guardians":[` + guardian + `]}`, `{"contact_phone":"bad"}`, `{"contact_phone":123}`,
		`{"contact_phone":"0771234567","contact_phone":"0712345678"}`,
		`{"contact_phone":"0771234567","\u0063ontact_phone":"0712345678"}`,
		`{"guardians":[{"name":"A","name":"B","relationship":"Mother","contact_phone":"0712345678"}]}`,
		`{"guardians":[null]}`, `{"guardians":[{"name":"Guardian","relationship":"Mother","contact_phone":"0712345678","guardian_id":"forged"}]}`,
		`{"contact_phone":"0771234567","user_id":"forged"}`, `{"student_id":"forged"}`, `{"email":"other@example.invalid"}`,
		`{"CONTACT_PHONE":"0771234567"}`, `{"contact_phone":"0771234567","Contact_Phone":"0712345678"}`,
		`{"guardians":[{"Name":"Guardian","relationship":"Mother","contact_phone":"0712345678"}]}`,
		`{"year":4}`, `{"faculty":"Science"}`, `{"index_no":"other"}`, `{"full_name":"Other"}`, `{"presence":{"status":"IN"}}`,
		`{"role":"admin"}`, `{"profile_image_url":"forged"}`, `{"accommodation":{}}`, string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}),
		`{"guardians":` + strings.Repeat("[", 34) + `0` + strings.Repeat("]", 34) + `}`,
	} {
		f := &ownEditFake{}
		mux := http.NewServeMux()
		student.RegisterOwnProfileEdit(h.NewOwnProfileEditAPI(s.NewOwnProfileEditor(f), time.Second), mux, m.Middleware(verifier{}, accounts{"student", true}, time.Second))
		r := httptest.NewRequest("PATCH", "/api/v1/me/student-profile", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer test")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 400 || f.calls != 0 {
			t.Fatalf("body %q: %d %s calls%d", body, w.Code, w.Body, f.calls)
		}
	}
}
