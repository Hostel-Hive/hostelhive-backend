package firebase

import (
	"context"
	"encoding/json"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFCMSDKPayloadErrorsAndRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		code           int
		fcmCode, state string
	}{{200, "", "accepted"}, {404, "UNREGISTERED", "invalid_token"}, {400, "INVALID_ARGUMENT", "failed"}, {403, "SENDER_ID_MISMATCH", "failed"}, {429, "QUOTA_EXCEEDED", "retry"}, {503, "UNAVAILABLE", "retry"}} {
		t.Run(tc.fcmCode+tc.state, func(t *testing.T) {
			calls := 0
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "fcm.googleapis.com" || !strings.HasSuffix(r.URL.Path, "/projects/hostelhive-test/messages:send") {
					t.Fatal("unexpected FCM destination", r.URL)
				}
				var request struct {
					Message map[string]json.RawMessage `json:"message"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Fatal("invalid SDK body")
				}
				var data map[string]string
				if json.Unmarshal(request.Message["data"], &data) != nil || len(data) != 2 || data["notification_id"] != "12345678-1234-4123-8123-123456789abc" || data["source_type"] != "leave" {
					t.Fatal("wrong/unsafe payload", data)
				}
				var note map[string]string
				_ = json.Unmarshal(request.Message["notification"], &note)
				if note["title"] != "HostelHive" || strings.Contains(note["body"], "approved") {
					t.Fatal("private content in notification", note)
				}
				var android struct {
					Priority     string `json:"priority"`
					TTL          string `json:"ttl"`
					Notification struct {
						Tag string `json:"tag"`
					} `json:"notification"`
				}
				if json.Unmarshal(request.Message["android"], &android) != nil || android.Priority != "normal" || android.TTL == "" || android.Notification.Tag != data["notification_id"] {
					t.Fatal(android)
				}
				text := `{"name":"projects/hostelhive-test/messages/accepted"}`
				if tc.code != 200 {
					text = `{"error":{"code":` + strconv.Itoa(tc.code) + `,"status":"INVALID_ARGUMENT","message":"provider text must never be exposed","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"` + tc.fcmCode + `"}]}}`
				}
				retryAfter := "180"
				if tc.code == 503 {
					retryAfter = "60"
				}
				return &http.Response{StatusCode: tc.code, Header: http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{retryAfter}}, Body: io.NopCloser(strings.NewReader(text)), Request: r}, nil
			})
			app, err := firebase.NewApp(context.Background(), &firebase.Config{ProjectID: "hostelhive-test"}, option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "offline"})), option.WithHTTPClient(&http.Client{Transport: pushRetryTransport{transport}}))
			if err != nil {
				t.Fatal(err)
			}
			client, err := app.Messaging(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			result := (&FCMSender{client}).Send(ctx, domain.PushMessage{Token: strings.Repeat("T", 50), NotificationID: "12345678-1234-4123-8123-123456789abc", SourceType: "leave", Kind: "leave_decided", ExpiresAt: time.Now().Add(time.Hour)})
			if result.State != tc.state || calls < 1 {
				t.Fatal(tc, result, calls)
			}
			if tc.code == 200 && result.ProviderID == "" {
				t.Fatal(result)
			}
			if tc.code >= 429 && result.RetryAfter < map[int]time.Duration{429: 170 * time.Second, 503: 50 * time.Second}[tc.code] {
				t.Fatal("lost Retry-After", result)
			}
		})
	}
}

type capturePushClient struct {
	calls   int
	message *messaging.Message
}

func (c *capturePushClient) Send(_ context.Context, m *messaging.Message) (string, error) {
	c.calls++
	c.message = m
	return "projects/test/messages/ok", nil
}
func TestFCMPriorityExpiryAndAPNS(t *testing.T) {
	for _, kind := range []string{"leave_decided", "late_access_decided", "attendance_incident"} {
		t.Run(kind, func(t *testing.T) {
			client := &capturePushClient{}
			expires := time.Now().Add(10 * time.Minute)
			result := (&FCMSender{client}).Send(context.Background(), domain.PushMessage{Token: "private-token", NotificationID: "opaque-notification-id", SourceType: "test", Kind: kind, ExpiresAt: expires})
			if result.State != "accepted" || client.calls != 1 {
				t.Fatal(result, client.calls)
			}
			m := client.message
			priority := "high"
			if kind == "leave_decided" {
				priority = "normal"
			}
			if m.Android.Priority != priority || m.Android.TTL == nil || *m.Android.TTL > 10*time.Minute || *m.Android.TTL <= 9*time.Minute {
				t.Fatal("priority/expiry", m.Android)
			}
			h := m.APNS.Headers
			if h["apns-push-type"] != "alert" || h["apns-priority"] != "10" || h["apns-collapse-id"] != m.Data["notification_id"] || h["apns-expiration"] != strconv.FormatInt(expires.Unix(), 10) {
				t.Fatal("APNs contract", h)
			}
		})
	}
	client := &capturePushClient{}
	result := (&FCMSender{client}).Send(context.Background(), domain.PushMessage{ExpiresAt: time.Now().Add(-time.Second)})
	if result.State != "failed" || result.Code != "expired" || client.calls != 0 {
		t.Fatal("expired message sent", result, client.calls)
	}
}
