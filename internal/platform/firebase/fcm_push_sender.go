package firebase

import (
	"context"
	"errors"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/errorutils"
	"firebase.google.com/go/v4/messaging"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type messagingClient interface {
	Send(context.Context, *messaging.Message) (string, error)
}
type FCMSender struct{ client messagingClient }

func NewFCMSender(ctx context.Context, projectID string) (*FCMSender, error) {
	// Credential refresh uses its construction context, so bound its HTTP client
	// as well as the individual FCM request deadline.
	credentialCtx := context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 5 * time.Second})
	clientHTTP, err := google.DefaultClient(credentialCtx, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, errors.New("initialize FCM: check Application Default Credentials")
	}
	clientHTTP.Timeout = 5 * time.Second
	transport := clientHTTP.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clientHTTP.Transport = pushRetryTransport{transport}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID}, option.WithHTTPClient(clientHTTP))
	if err != nil {
		return nil, errors.New("initialize FCM: check project and credentials")
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, errors.New("initialize FCM: check Application Default Credentials")
	}
	return &FCMSender{client}, nil
}
func (f *FCMSender) Send(ctx context.Context, v domain.PushMessage) domain.PushResult {
	ttl := min(time.Until(v.ExpiresAt), time.Hour)
	if ttl <= 0 {
		return domain.PushResult{State: "failed", Code: "expired"}
	}
	priority := "normal"
	if v.Kind == "late_access_decided" || v.Kind == "attendance_incident" {
		priority = "high"
	}
	message := &messaging.Message{
		Token:        v.Token,
		Notification: &messaging.Notification{Title: "HostelHive", Body: "You have a new notification. Open HostelHive to view it."},
		Data:         map[string]string{"notification_id": v.NotificationID, "source_type": v.SourceType},
		Android:      &messaging.AndroidConfig{Priority: priority, TTL: &ttl, Notification: &messaging.AndroidNotification{Tag: v.NotificationID}},
		APNS:         &messaging.APNSConfig{Headers: map[string]string{"apns-push-type": "alert", "apns-priority": "10", "apns-expiration": strconv.FormatInt(v.ExpiresAt.Unix(), 10), "apns-collapse-id": v.NotificationID}},
	}
	attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	capture := &pushRetryCapture{}
	attempt = context.WithValue(attempt, pushRetryKey{}, capture)
	id, err := f.client.Send(attempt, message)
	if err == nil && id != "" {
		return domain.PushResult{State: "accepted", ProviderID: id}
	}
	result := domain.PushResult{State: "retry", Code: "provider_unavailable"}
	switch {
	case messaging.IsUnregistered(err):
		result.State, result.Code = "invalid_token", "unregistered"
	case messaging.IsInvalidArgument(err), messaging.IsSenderIDMismatch(err):
		result.State, result.Code = "failed", "provider_rejected"
	}
	// INVALID_ARGUMENT is not proof that the device token itself is invalid.
	if response := errorutils.HTTPResponse(err); response != nil {
		result.RetryAfter = pushRetryAfter(response.Header.Get("Retry-After"), time.Now())
	}
	capture.mu.Lock()
	if delay := time.Until(capture.until); delay > result.RetryAfter {
		result.RetryAfter = delay
	}
	capture.mu.Unlock()
	return result
}
func pushRetryAfter(value string, now time.Time) time.Duration {
	if n, err := strconv.ParseInt(value, 10, 32); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

// Preserve Retry-After even if an SDK-internal 503 retry ends at our deadline.
type pushRetryKey struct{}
type pushRetryCapture struct {
	mu    sync.Mutex
	until time.Time
}
type pushRetryTransport struct{ base http.RoundTripper }

func (t pushRetryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if response != nil {
		if capture, ok := r.Context().Value(pushRetryKey{}).(*pushRetryCapture); ok {
			now := time.Now()
			until := now.Add(pushRetryAfter(response.Header.Get("Retry-After"), now))
			capture.mu.Lock()
			if until.After(capture.until) {
				capture.until = until
			}
			capture.mu.Unlock()
		}
	}
	return response, err
}
