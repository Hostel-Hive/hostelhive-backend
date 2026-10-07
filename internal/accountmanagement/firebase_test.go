package accountmanagement

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	firebase "firebase.google.com/go/v4"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFirebaseSDKDisableAndRevoke(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "")
	for _, mode := range []string{"success", "disable-failed", "revoke-failed", "not-found", "wrong-uid", "not-disabled"} {
		t.Run(mode, func(t *testing.T) {
			updates := 0
			disables := 0
			revokes := 0
			transport := transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "identitytoolkit.googleapis.com" {
					t.Fatal("unexpected destination")
				}
				var b map[string]any
				if json.NewDecoder(r.Body).Decode(&b) != nil {
					t.Fatal("bad SDK payload")
				}
				status := 200
				response := `{"localId":"target-uid","disabled":true}`
				if strings.HasSuffix(r.URL.Path, "/accounts:lookup") {
					ids, ok := b["localId"].([]any)
					if !ok || len(ids) != 1 || ids[0] != "target-uid" {
						t.Fatal("wrong lookup")
					}
					response = `{"users":[{"localId":"target-uid","disabled":true}]}`
					if mode == "wrong-uid" {
						response = `{"users":[{"localId":"unrelated-uid","disabled":true}]}`
					}
					if mode == "not-disabled" {
						response = `{"users":[{"localId":"target-uid","disabled":false}]}`
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
				}
				if !strings.HasSuffix(r.URL.Path, "/accounts:update") || b["localId"] != "target-uid" {
					t.Fatal("wrong UID or endpoint")
				}
				updates++
				if b["disableUser"] == true {
					disables++
					switch mode {
					case "disable-failed":
						status = 500
						response = `{"error":{"message":"INTERNAL_ERROR"}}`
					case "not-found":
						status = 400
						response = `{"error":{"message":"USER_NOT_FOUND"}}`
					case "wrong-uid":
						response = `{"localId":"unrelated-uid","disabled":true}`
					case "not-disabled":
						response = `{"localId":"target-uid","disabled":false}`
					}
				}
				if _, ok := b["validSince"]; ok {
					revokes++
					if mode == "revoke-failed" {
						status = 500
						response = `{"error":{"message":"INTERNAL_ERROR"}}`
					}
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
			err = NewFirebaseRevoker(client).DisableAndRevoke(context.Background(), "target-uid")
			success := mode == "success" || mode == "not-found"
			if (err == nil) != success || disables != 1 {
				t.Fatalf("mode=%s err=%v disables=%d", mode, err, disables)
			}
			want := 0
			if mode == "success" || mode == "revoke-failed" {
				want = 1
			}
			if revokes != want || updates != disables+revokes {
				t.Fatalf("revocation ordering wrong: %d", revokes)
			}
		})
	}
}
