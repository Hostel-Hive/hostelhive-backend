package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type allowanceReader struct {
	calls    int
	year     int
	actor    string
	deadline bool
	err      error
}

func (f *allowanceReader) GetAllowance(ctx context.Context, actor string, year int) (domain.AllowanceSummary, error) {
	f.calls++
	f.actor, f.year = actor, year
	_, f.deadline = ctx.Deadline()
	return domain.AllowanceSummary{StudentID: "own-student", Year: year, Timezone: "Asia/Colombo", TotalDays: 18, UsedDays: 7, RemainingDays: 11}, f.err
}

func TestAllowanceHTTP(t *testing.T) {
	f := &allowanceReader{}
	account := identity.Account{FirebaseUID: "student", Role: "student", IsActive: true}
	invoke := func(method, path, body, token string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		// Retained historical calculator, not the retired public route.
		auth := middleware.Middleware(goingHomeVerifier{}, goingHomeAccounts{account}, time.Second)
		mux.Handle("GET /api/v1/me/leave-allowance", auth(middleware.RequireRoles("student")(http.HandlerFunc(handler.NewAllowance(f, time.Second).Get))))
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	const path = "/api/v1/me/leave-allowance"
	w := invoke("GET", path+"?year=2026", "", "student")
	var v domain.AllowanceSummary
	if json.Unmarshal(w.Body.Bytes(), &v) != nil || w.Code != 200 || v.TotalDays != 18 || v.UsedDays != 7 || v.RemainingDays != 11 || w.Header().Get("Cache-Control") != "no-store" || f.year != 2026 || f.actor != "student" || !f.deadline {
		t.Fatal(w.Code, w.Body, f)
	}
	if w = invoke("GET", path, "", "student"); w.Code != 200 || f.year != 0 {
		t.Fatal(w.Code, w.Body)
	}
	for _, year := range []string{"0001", "9998"} {
		if w := invoke("GET", path+"?year="+year, "", "student"); w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	before := f.calls
	for _, query := range []string{"year=", "year=0", "year=0000", "year=9999", "year=10000", "year=-001", "year=2026&year=2027", "year=%2B026", "year=202x", "year=2026&limit=1", "student_id=other", "user_id=other", "year=%zz", "year=2026;unknown=1"} {
		if w := invoke("GET", path+"?"+query, "", "student"); w.Code != 400 {
			t.Fatal(query, w.Code, w.Body)
		}
	}
	for _, body := range []string{"{}", "null", " "} {
		if w := invoke("GET", path, body, "student"); w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if w := invoke(method, path, "", "student"); w.Code != 405 {
			t.Fatal(method, w.Code, w.Body)
		}
	}
	for _, token := range []string{"", "bad"} {
		if w := invoke("GET", path, "", token); w.Code != 401 {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff"} {
		account.Role = role
		if w := invoke("GET", path, "", "student"); w.Code != 403 {
			t.Fatal(role, w.Code, w.Body)
		}
	}
	account.Role, account.IsActive = "student", false
	if w := invoke("GET", path, "", "student"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	account.IsActive, account.SetupRequired = true, true
	if w := invoke("GET", path, "", "student"); w.Code != 403 || !strings.Contains(w.Body.String(), "setup_required") {
		t.Fatal(w.Code, w.Body)
	}
	if f.calls != before {
		t.Fatal("invalid or unauthorized input reached the allowance reader")
	}
	account.SetupRequired = false
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalid, 400, "invalid_input"},
		{domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrIneligible, 409, "active_student_required"},
		{errors.New("private database details"), 503, "leave_unavailable"},
	} {
		f.err = tc.err
		w := invoke("GET", path, "", "student")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "private database details") {
			t.Fatal(w.Code, w.Body)
		}
	}
}
