package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type restorationFake struct {
	calls     int
	err       error
	actor, id string
}

func (f *restorationFake) ListArchived(ctx context.Context, actor string, filter d.Filter) (d.Page, error) {
	f.calls++
	f.actor = actor
	return d.Page{Students: []d.Profile{}, Limit: filter.Limit, Offset: filter.Offset}, f.err
}

func (f *restorationFake) Restore(ctx context.Context, actor, id string) (d.RestorationResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing timeout")
	}
	f.calls++
	f.actor, f.id = actor, id
	return d.RestorationResult{Student: d.Profile{StudentID: id}, Restored: true}, f.err
}
func restorationHandler(f *restorationFake, role string, active bool) http.Handler {
	mux := http.NewServeMux()
	student.RegisterRestoration(h.NewRestorationAPI(f, time.Second), mux, m.Middleware(verifier{}, accounts{role, active}, time.Second))
	return mux
}
func TestRestorationAdminOnly(t *testing.T) {
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		f := &restorationFake{}
		res := request(restorationHandler(f, role, true), "POST", "/api/v1/students/"+testID+"/restore", "", true)
		expected := 403
		if role == "admin" {
			expected = 200
		}
		if res.Code != expected || (role != "admin" && f.calls != 0) {
			t.Fatalf("role %s: %d", role, res.Code)
		}
		if role == "admin" {
			var result d.RestorationResult
			if json.Unmarshal(res.Body.Bytes(), &result) != nil || !result.Restored || f.actor != "verified-admin" || f.id != testID || res.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("restore result", res.Body.String())
			}
		}
	}
	for _, token := range []bool{false, true} {
		f := &restorationFake{}
		res := request(restorationHandler(f, "admin", false), "POST", "/api/v1/students/"+testID+"/restore", "", token)
		if (res.Code != 401 && res.Code != 403) || f.calls != 0 {
			t.Fatal("inactive/anonymous restore", res.Code)
		}
	}
}
func TestRestorationValidationAndFailures(t *testing.T) {
	for _, tc := range []struct{ path, body string }{
		{"/api/v1/students/bad/restore", ""},
		{"/api/v1/students/" + testID + "/restore?force=true", ""},
		{"/api/v1/students/" + testID + "/restore", "{}"},
		{"/api/v1/students/" + testID + "/restore", " "},
	} {
		f := &restorationFake{}
		res := request(restorationHandler(f, "admin", true), "POST", tc.path, tc.body, true)
		if res.Code != 400 || f.calls != 0 {
			t.Fatal("invalid request reached restore", res.Code)
		}
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{d.ErrNotFound, 404}, {d.ErrAccount, 409}, {d.ErrConflict, 409}, {d.ErrForbidden, 403}, {d.ErrUnavailable, 503}} {
		f := &restorationFake{err: tc.err}
		res := request(restorationHandler(f, "admin", true), "POST", "/api/v1/students/"+testID+"/restore", "", true)
		if res.Code != tc.status {
			t.Fatal(tc.err, res.Code)
		}
	}
}

func TestArchivedDirectoryAdminOnly(t *testing.T) {
	for _, role := range []string{"admin", "warden", "student", "security_staff", "sub_warden"} {
		f := &restorationFake{}
		res := request(restorationHandler(f, role, true), "GET", "/api/v1/students/archived?faculty=Science&limit=20", "", true)
		expected := 403
		if role == "admin" {
			expected = 200
		}
		if res.Code != expected || (role != "admin" && f.calls != 0) {
			t.Fatal("archived directory policy", role, res.Code)
		}
	}
	for _, path := range []string{"/api/v1/students/archived?limit=0", "/api/v1/students/archived?unknown=true"} {
		f := &restorationFake{}
		res := request(restorationHandler(f, "admin", true), "GET", path, "", true)
		if res.Code != 400 || f.calls != 0 {
			t.Fatal("invalid archived filter", res.Code)
		}
	}
}
