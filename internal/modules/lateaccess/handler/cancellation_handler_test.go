package handler_test

import (
	"context"
	"errors"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	feature "github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/lateaccess/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type cancelStore struct {
	calls     int
	err       error
	actor, id string
}

func (f *cancelStore) Cancel(_ context.Context, actor, id string) (domain.Request, error) {
	f.calls++
	f.actor, f.id = actor, id
	return domain.Request{Status: "cancelled"}, f.err
}
func (f *cancelStore) Submit(context.Context, string, dto.Submit) (domain.Request, error) {
	panic("unexpected submit")
}
func (f *cancelStore) Decide(context.Context, string, string, dto.Decision) (domain.Request, error) {
	panic("unexpected decision")
}
func (f *cancelStore) Get(context.Context, string, string) (domain.Request, error) {
	panic("unexpected detail")
}
func (f *cancelStore) List(context.Context, string, domain.Filter) (domain.Page, error) {
	panic("unexpected list")
}

type cancelVerifier struct{}

func (cancelVerifier) Verify(_ context.Context, token string) (string, error) { return token, nil }

type cancelAccounts struct{ account identity.Account }

func (f cancelAccounts) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return f.account, nil
}
func TestCancelHTTP(t *testing.T) {
	f := &cancelStore{}
	account := identity.Account{FirebaseUID: "student", Role: "student", IsActive: true}
	invoke := func(path, body, token string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		feature.Register(handler.New(service.New(f), time.Second), mux, middleware.Middleware(cancelVerifier{}, cancelAccounts{account}, time.Second))
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	path := "/api/v1/late-access-requests/AB000000-0000-4000-8000-000000000001/cancel"
	w := invoke(path, "", "student")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || f.actor != "student" || f.id != "ab000000-0000-4000-8000-000000000001" {
		t.Fatal(w.Code, w.Body, f)
	}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalid, 400, "invalid_input"}, {domain.ErrForbidden, 403, "forbidden"}, {domain.ErrNotFound, 404, "not_found"},
		{domain.ErrIneligible, 409, "active_student_required"}, {domain.ErrConflict, 409, "late_access_conflict"}, {errors.New("private database error"), 503, "late_access_unavailable"},
	} {
		f.err = tc.err
		w = invoke(path, "", "student")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "private database error") {
			t.Fatal(w.Code, w.Body)
		}
	}
	f.err = nil
	before := f.calls
	for _, tc := range []struct {
		path, body, token string
		want              int
	}{
		{path + "?student_id=other", "", "student", 400}, {path, "{}", "student", 400}, {path, "null", "student", 400},
		{"/api/v1/late-access-requests/bad/cancel", "", "student", 400}, {path, "", "", 401},
	} {
		if w := invoke(tc.path, tc.body, tc.token); w.Code != tc.want {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff"} {
		account.Role = role
		if w := invoke(path, "", "staff"); w.Code != 403 {
			t.Fatal(role, w.Code)
		}
	}
	account.Role = "student"
	account.IsActive = false
	if w := invoke(path, "", "student"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if f.calls != before {
		t.Fatal("invalid/unauthorized request reached persistence")
	}
}
