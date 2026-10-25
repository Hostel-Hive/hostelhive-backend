package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	h "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

const validCheckoutJSON = `{"request_id":"22222222-2222-4222-8222-222222222222","reason":"graduated","deactivate_account":false,"confirmed":true}`

type checkoutHTTPFake struct {
	calls int
	err   error
}

func (f *checkoutHTTPFake) Checkout(ctx context.Context, actor, id string, in dto.CheckoutInput, hash []byte) (d.CheckoutResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing timeout")
	}
	f.calls++
	return d.CheckoutResult{Checkout: d.Checkout{StudentID: id, RequestID: in.RequestID}}, f.err
}
func checkoutHandler(f *checkoutHTTPFake, role string, active bool) http.Handler {
	mux := http.NewServeMux()
	student.RegisterCheckout(h.NewCheckoutAPI(svc.NewCheckouts(f), time.Second), mux, m.Middleware(verifier{}, accounts{role, active}, time.Second))
	return mux
}
func checkoutRequest(handler http.Handler, path, raw, media string, token bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(raw))
	r.Header.Set("Content-Type", media)
	if token {
		r.Header.Set("Authorization", "Bearer token")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func TestCheckoutHTTPAuthorizationAndSuccess(t *testing.T) {
	for _, role := range []string{"admin", "warden", "sub_warden", "security_staff", "student"} {
		f := &checkoutHTTPFake{}
		w := checkoutRequest(checkoutHandler(f, role, true), "/api/v1/students/"+testID+"/checkout", validCheckoutJSON, "application/json", true)
		expected := 403
		if role == "admin" {
			expected = 200
		}
		if w.Code != expected || (role != "admin" && f.calls != 0) {
			t.Fatal(role, w.Code)
		}
		if role == "admin" {
			var result d.CheckoutResult
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Checkout.StudentID != testID || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(w.Body.String())
			}
		}
	}
	for _, active := range []bool{false, true} {
		f := &checkoutHTTPFake{}
		w := checkoutRequest(checkoutHandler(f, "admin", active), "/api/v1/students/"+testID+"/checkout", validCheckoutJSON, "application/json", !active)
		if (w.Code != 401 && w.Code != 403) || f.calls != 0 {
			t.Fatal("inactive/anonymous checkout", w.Code)
		}
	}
}
func TestCheckoutHTTPInvalidAndFailures(t *testing.T) {
	path := "/api/v1/students/" + testID + "/checkout"
	for _, tc := range []struct {
		path, raw, media string
		status           int
	}{
		{path, validCheckoutJSON, "text/plain", 415}, {path + "?force=true", validCheckoutJSON, "application/json", 400},
		{"/api/v1/students/bad/checkout", validCheckoutJSON, "application/json", 400},
		{path, `{}`, "application/json", 400}, {path, validCheckoutJSON + `{}`, "application/json", 400},
		{path, strings.Replace(validCheckoutJSON, `"confirmed":true`, `"confirmed":false`, 1), "application/json", 400},
		{path, strings.Replace(validCheckoutJSON, `"deactivate_account":false`, `"deactivate_account":null`, 1), "application/json", 400},
	} {
		f := &checkoutHTTPFake{}
		w := checkoutRequest(checkoutHandler(f, "admin", true), tc.path, tc.raw, tc.media, true)
		if w.Code != tc.status || f.calls != 0 {
			t.Fatal("invalid", w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{d.ErrCheckoutConflict, 409, "checkout_conflict"}, {d.ErrAlreadyCheckedOut, 409, "student_already_checked_out"}, {d.ErrCheckoutIneligible, 409, "student_account_required"},
		{d.ErrNotFound, 404, "not_found"}, {d.ErrForbidden, 403, "forbidden"}, {d.ErrUnavailable, 503, "student_profiles_unavailable"},
	} {
		f := &checkoutHTTPFake{err: tc.err}
		w := checkoutRequest(checkoutHandler(f, "admin", true), path, validCheckoutJSON, "application/json", true)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatal(tc.err, w.Code, w.Body.String())
		}
	}
}
