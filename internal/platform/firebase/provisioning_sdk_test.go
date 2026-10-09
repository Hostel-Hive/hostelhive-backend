package firebase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	firebase "firebase.google.com/go/v4"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise the actual pinned SDK's request serialization and error mapping
// through an in-memory transport; no credentials or real Firebase users.

func TestFirebaseSDKProvisioningRequests(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "")
	for _, mode := range []string{"create", "uid-race", "email-conflict", "invalid-email"} {
		t.Run(mode, func(t *testing.T) {
			exists, disabled := false, true
			creates := 0
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "identitytoolkit.googleapis.com" {
					t.Fatalf("unexpected SDK request destination: %s", r.URL.Host)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				status := 200
				response := "{}"
				switch {
				case strings.HasSuffix(r.URL.Path, "/accounts:lookup"):
					ids, ok := body["localId"].([]any)
					if !ok || len(ids) != 1 || ids[0] != "reserved-uid" {
						t.Fatal("SDK lookup must use the reserved UID")
					}
					if exists {
						response = `{"users":[{"localId":"reserved-uid","email":"user@example.invalid","disabled":` + map[bool]string{true: "true", false: "false"}[disabled] + `}]} `
					} else {
						response = `{"users":[]}`
					}
				case strings.HasSuffix(r.URL.Path, "/accounts"):
					creates++
					if body["localId"] != "reserved-uid" || body["email"] != "user@example.invalid" || body["password"] != "Local-test-password-20" || body["disabled"] != true {
						t.Fatal("identity was not created disabled with reserved UID")
					}
					switch mode {
					case "email-conflict":
						status = 400
						response = `{"error":{"message":"EMAIL_EXISTS"}}`
					case "invalid-email":
						status = 400
						response = `{"error":{"message":"INVALID_EMAIL"}}`
					case "uid-race":
						exists = true
						status = 400
						response = `{"error":{"message":"DUPLICATE_LOCAL_ID"}}`
					default:
						exists = true
						response = `{"localId":"reserved-uid"}`
					}
				case strings.HasSuffix(r.URL.Path, "/accounts:update"):
					if body["localId"] != "reserved-uid" || body["disableUser"] != false {
						t.Fatal("wrong activation request")
					}
					if _, ok := body["password"]; ok {
						t.Fatal("activation reset the password")
					}
					disabled = false
				default:
					t.Fatalf("unexpected API path: %s", r.URL.Path)
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
			})
			app, err := firebase.NewApp(context.Background(), &firebase.Config{ProjectID: "hostelhive-test", ServiceAccountID: "test@example.invalid"}, option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "offline-test"})), option.WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			client, err := app.Auth(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			identities := NewFirebaseIdentities(client)
			err = identities.EnsureDisabled(context.Background(), "reserved-uid", "user@example.invalid", "Local-test-password-20")
			if mode == "email-conflict" {
				if !errors.Is(err, domain.ErrProvisionConflict) {
					t.Fatal(err)
				}
				return
			}
			if mode == "invalid-email" {
				if !errors.Is(err, domain.ErrProvisionInvalidInput) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = identities.Enable(context.Background(), "reserved-uid", "user@example.invalid"); err != nil {
				t.Fatal(err)
			}
			if disabled {
				t.Fatal("Firebase identity stayed disabled")
			}
			if err = identities.EnsureDisabled(context.Background(), "reserved-uid", "user@example.invalid", "Changed-retry-password"); err != nil || creates != 1 {
				t.Fatal("retry duplicated creation or changed password")
			}
		})
	}
}
