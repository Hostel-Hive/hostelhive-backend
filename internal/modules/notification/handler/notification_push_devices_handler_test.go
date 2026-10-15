package handler_test

import (
	"context"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type pushVerifier struct{}

func (pushVerifier) Verify(_ context.Context, v string) (string, error) { return v, nil }

type pushAccounts struct{}

func (pushAccounts) FindByFirebaseUID(_ context.Context, v string) (identity.Account, error) {
	return identity.Account{FirebaseUID: v, Role: v, IsActive: true}, nil
}

type pushDevicesFake struct {
	err   error
	calls int
}

func (f *pushDevicesFake) RegisterPushDevice(_ context.Context, _ string, id string, v dto.PushRegistration) (domain.PushDevice, error) {
	f.calls++
	return domain.PushDevice{DeviceID: id, Platform: v.Platform, Active: true}, f.err
}
func (f *pushDevicesFake) ListPushDevices(context.Context, string) (domain.PushDevicePage, error) {
	f.calls++
	return domain.PushDevicePage{Items: []domain.PushDevice{}}, f.err
}
func (f *pushDevicesFake) RemovePushDevice(context.Context, string, string) error {
	f.calls++
	return f.err
}
func TestPushDeviceHTTPContracts(t *testing.T) {
	f := &pushDevicesFake{}
	mux := http.NewServeMux()
	notification.RegisterPush(handler.NewPush(service.NewPush(f), time.Second), mux, middleware.Middleware(pushVerifier{}, pushAccounts{}, time.Second))
	id := "12345678-1234-4123-8123-123456789abc"
	base := "/api/v1/me/push-devices"
	valid := `{"platform":"android","token":"` + strings.Repeat("T", 50) + `"}`
	call := func(method, path, body, media, role string, want int) {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if role != "" {
			r.Header.Set("Authorization", "Bearer "+role)
		}
		if media != "" {
			r.Header.Set("Content-Type", media)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(method, path, w.Code, w.Body.String(), w.Header())
		}
		if strings.Contains(w.Body.String(), strings.Repeat("T", 50)) || strings.Contains(w.Body.String(), "token_hash") {
			t.Fatal("registration secret leaked")
		}
	}
	for _, role := range []string{"admin", "warden", "sub_warden", "student", "security_staff"} {
		call("PUT", base+"/"+id, valid, "application/json", role, 200)
		call("GET", base, "", "", role, 200)
		call("DELETE", base+"/"+id, "", "", role, 204)
	}
	for _, bad := range []string{`{}`, `null`, `[]`, `{"platform":"web","token":"` + strings.Repeat("T", 50) + `"}`, `{"platform":"android","platform":"ios","token":"` + strings.Repeat("T", 50) + `"}`, `{"platform":"android","token":"short"}`, valid + ` {}`, `{"platform":"android","token":"` + strings.Repeat("T", 17000) + `"}`, `{"platform":"ios","token":"` + strings.Repeat("T", 50) + `","user_id":"other"}`} {
		call("PUT", base+"/"+id, bad, "application/json", "student", 400)
	}
	call("PUT", base+"/"+id, valid, "text/plain", "student", 415)
	call("PUT", base+"/invalid", valid, "application/json", "student", 400)
	call("GET", base+"?user_id=other", "", "", "student", 400)
	call("GET", base, "{}", "application/json", "student", 400)
	call("DELETE", base+"/"+id, "{}", "application/json", "student", 400)
	call("GET", base, "", "", "", 401)
	for _, tc := range []struct {
		err  error
		want int
	}{{domain.ErrPushConflict, 409}, {domain.ErrForbidden, 403}, {domain.ErrUnavailable, 503}, {domain.ErrNotFound, 404}} {
		f.err = tc.err
		call("PUT", base+"/"+id, valid, "application/json", "student", tc.want)
	}
}
