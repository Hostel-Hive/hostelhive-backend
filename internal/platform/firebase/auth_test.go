package firebase

import (
	"context"
	"errors"
	"strings"
	"testing"

	firebaseauth "firebase.google.com/go/v4/auth"
)

type firebaseClientFunc func(context.Context, string) (*firebaseauth.Token, error)

func (f firebaseClientFunc) VerifyIDTokenAndCheckRevoked(ctx context.Context, raw string) (*firebaseauth.Token, error) {
	return f(ctx, raw)
}

func TestFirebaseVerifierChecksRevocation(t *testing.T) {
	client := firebaseClientFunc(func(ctx context.Context, raw string) (*firebaseauth.Token, error) {
		if raw != "id-token" {
			t.Fatal("wrong token")
		}
		return &firebaseauth.Token{UID: "verified-uid", Claims: map[string]interface{}{"role": "admin"}}, nil
	})
	verifier := &FirebaseVerifier{client: client}
	uid, err := verifier.Verify(context.Background(), "id-token")
	if err != nil || uid != "verified-uid" {
		t.Fatalf("%s %v", uid, err)
	}
	for _, client := range []firebaseClientFunc{
		func(context.Context, string) (*firebaseauth.Token, error) {
			return nil, errors.New("private SDK error")
		},
		func(context.Context, string) (*firebaseauth.Token, error) { return nil, nil },
		func(context.Context, string) (*firebaseauth.Token, error) { return &firebaseauth.Token{}, nil },
	} {
		_, err := (&FirebaseVerifier{client: client}).Verify(context.Background(), "id-token")
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("verification failed open or leaked detail")
		}
	}
}

func TestFirebaseEmulatorRejected(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "127.0.0.1:9099")
	if _, err := NewFirebaseVerifier(context.Background(), "hostelhive-test"); err == nil {
		t.Fatal("unsigned emulator authentication allowed")
	}
}

func TestFirebaseCredentialErrorRedacted(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "C:/missing-private-credentials/issue15-secret.json")
	_, err := NewFirebaseVerifier(context.Background(), "hostelhive-test")
	if err == nil || strings.Contains(err.Error(), "issue15-secret") || strings.Contains(err.Error(), "missing-private") {
		t.Fatalf("unsafe startup error: %v", err)
	}
}
