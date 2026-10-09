package firebase

import (
	"context"
	"encoding/json"
	"errors"
	firebase "firebase.google.com/go/v4"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFirebaseSDKReactivation(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "")
	for _, mode := range []string{"success", "missing", "email-mismatch", "enable-failed", "still-disabled"} {
		t.Run(mode, func(t *testing.T) {
			revoked, enabled := false, false
			transport := transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "identitytoolkit.googleapis.com" {
					t.Fatal("unexpected destination")
				}
				var b map[string]any
				if json.NewDecoder(r.Body).Decode(&b) != nil {
					t.Fatal("bad payload")
				}
				status := 200
				response := `{"localId":"target-uid"}`
				if strings.HasSuffix(r.URL.Path, "/accounts:lookup") {
					email := "user@example.invalid"
					if mode == "email-mismatch" {
						email = "other@example.invalid"
					}
					response = `{"users":[{"localId":"target-uid","email":"` + email + `","disabled":` + map[bool]string{true: "false", false: "true"}[enabled && mode != "still-disabled"] + `}]}`
					if mode == "missing" {
						response = `{"users":[]}`
					}
				} else if strings.HasSuffix(r.URL.Path, "/accounts:update") {
					if b["localId"] != "target-uid" {
						t.Fatal("wrong target")
					}
					if _, ok := b["validSince"]; ok {
						revoked = true
					} else if disabled, ok := b["disableUser"].(bool); ok && !disabled {
						if !revoked {
							t.Fatal("enabled before revocation")
						}
						enabled = true
						if mode == "enable-failed" {
							status = 400
							response = `{"error":{"message":"OPERATION_NOT_ALLOWED"}}`
						}
					} else {
						t.Fatal("unexpected mutation")
					}
				} else {
					t.Fatal("unexpected path")
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
			})
			app, err := firebase.NewApp(context.Background(), &firebase.Config{ProjectID: "hostelhive-test", ServiceAccountID: "test@example.invalid"}, option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "offline-test-only"})), option.WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			client, err := app.Auth(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			err = NewFirebaseReactivator(client).Reactivate(context.Background(), "target-uid", "user@example.invalid")
			if mode == "success" {
				if err != nil || !enabled || !revoked {
					t.Fatal("activation failed", err)
				}
			} else if err == nil {
				t.Fatal("false success")
			}
			if mode == "missing" && !errors.Is(err, domain.ErrIdentityMissing) {
				t.Fatal(err)
			}
			if (mode == "missing" || mode == "email-mismatch") && (enabled || revoked) {
				t.Fatal("unverified identity mutated")
			}
		})
	}
}
