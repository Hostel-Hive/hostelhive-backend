package leave_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type policyVerifier struct{}

func (policyVerifier) Verify(_ context.Context, token string) (string, error) { return token, nil }

type policyAccounts struct{}

func (policyAccounts) FindByFirebaseUID(_ context.Context, uid string) (identity.Account, error) {
	return identity.Account{FirebaseUID: uid, Role: uid, IsActive: true}, nil
}

func TestRetiredLeaveRoutes(t *testing.T) {
	mux := http.NewServeMux()
	auth := middleware.Middleware(policyVerifier{}, policyAccounts{}, time.Second)
	// Nil repositories prove retired routes never invoke old reads or mutations.
	leave.Register(handler.New(nil, time.Second), mux, auth)
	leave.RegisterAllowance(handler.NewAllowance(nil, time.Second), mux, auth)
	for _, tc := range []struct{ method, path, roles string }{
		{"POST", "/api/v1/leaves", "student"},
		{"POST", "/api/v1/leaves/11111111-1111-4111-8111-111111111111/cancel", "student"},
		{"POST", "/api/v1/leaves/11111111-1111-4111-8111-111111111111/decision", "admin,warden"},
		{"GET", "/api/v1/me/leave-allowance", "student"},
	} {
		for _, role := range []string{"", "student", "admin", "warden", "sub_warden", "security_staff"} {
			t.Run(tc.method+tc.path+role, func(t *testing.T) {
				r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"request_id":"obsolete"}`))
				if role != "" {
					r.Header.Set("Authorization", "Bearer "+role)
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				want := 403
				if role == "" {
					want = 401
				} else if strings.Contains(tc.roles, role) {
					want = 410
				}
				if w.Code != want {
					t.Fatalf("want %d got %d: %s", want, w.Code, w.Body)
				}
				if want == 410 {
					var result map[string]string
					if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["error"] != "leave_workflow_retired" || w.Header().Get("Cache-Control") != "no-store" {
						t.Fatalf("incorrect retirement response: %s", w.Body)
					}
				}
			})
		}
	}
}
