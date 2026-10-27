package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type goingHomeStore struct {
	calls    int
	err      error
	replayed bool
}

func (f *goingHomeStore) DeclareGoingHome(context.Context, string, dto.GoingHomeInput) (domain.GoingHomeDeclaration, bool, error) {
	f.calls++
	return domain.GoingHomeDeclaration{Status: "declared"}, f.replayed, f.err
}

type goingHomeVerifier struct{}

func (goingHomeVerifier) Verify(_ context.Context, token string) (string, error) {
	if token == "bad" {
		return "", errors.New("invalid token")
	}
	return token, nil
}

type goingHomeAccounts struct{ account identity.Account }

func (f goingHomeAccounts) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return f.account, nil
}
func TestGoingHomeHTTP(t *testing.T) {
	f := &goingHomeStore{}
	account := identity.Account{FirebaseUID: "student", Role: "student", IsActive: true}
	invoke := func(method, path, body, media, token string) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		leave.RegisterGoingHome(handler.NewGoingHome(f, time.Second), mux, middleware.Middleware(goingHomeVerifier{}, goingHomeAccounts{account}, time.Second))
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", media)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	valid := `{"request_id":"10000000-0000-4000-8000-000000000001"}`
	w := invoke("POST", "/api/v1/going-home", valid, "application/json", "student")
	if w.Code != 201 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body)
	}
	f.replayed = true
	if w = invoke("POST", "/api/v1/going-home", valid, "application/json", "student"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	f.replayed = false
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalid, 400, "invalid_input"}, {domain.ErrForbidden, 403, "forbidden"}, {domain.ErrIneligible, 409, "active_student_required"},
		{domain.ErrAlreadyOutside, 409, "already_outside"}, {domain.ErrPresenceUnknown, 409, "presence_unknown"}, {domain.ErrGoingHomePending, 409, "going_home_pending"},
		{domain.ErrConflict, 409, "going_home_conflict"}, {errors.New("private failure"), 503, "going_home_unavailable"},
	} {
		f.err = tc.err
		w = invoke("POST", "/api/v1/going-home", valid, "application/json", "student")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "private failure") {
			t.Fatal(w.Code, w.Body)
		}
	}
	f.err = nil
	for _, tc := range []struct {
		path, body, media string
		status            int
	}{
		{"/api/v1/going-home?student_id=other", valid, "application/json", 400},
		{"/api/v1/going-home", `{"request_id":"id","student_id":"other"}`, "application/json", 400},
		{"/api/v1/going-home", valid + ` {}`, "application/json", 400},
		{"/api/v1/going-home", ``, "application/json", 400},
		{"/api/v1/going-home", valid, "text/plain", 415},
		{"/api/v1/going-home", `{"request_id":"` + strings.Repeat("x", 17000) + `"}`, "application/json", 413},
	} {
		before := f.calls
		w = invoke("POST", tc.path, tc.body, tc.media, "student")
		if w.Code != tc.status || f.calls != before {
			t.Fatal(tc.status, w.Code, w.Body)
		}
	}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff"} {
		account.Role = role
		before := f.calls
		w = invoke("POST", "/api/v1/going-home", valid, "application/json", "student")
		if w.Code != 403 || f.calls != before {
			t.Fatal(role, w.Code)
		}
	}
	account.Role = "student"
	account.IsActive = false
	if w = invoke("POST", "/api/v1/going-home", valid, "application/json", "student"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	account.IsActive = true
	account.SetupRequired = true
	if w = invoke("POST", "/api/v1/going-home", valid, "application/json", "student"); w.Code != 403 || !strings.Contains(w.Body.String(), "setup_required") {
		t.Fatal(w.Code, w.Body)
	}
	account.SetupRequired = false
	for _, token := range []string{"", "bad"} {
		if w = invoke("POST", "/api/v1/going-home", valid, "application/json", token); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		if w = invoke(method, "/api/v1/going-home", "", "", "student"); w.Code != 405 {
			t.Fatal(method, w.Code)
		}
	}
}
