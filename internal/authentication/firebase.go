package authentication

import (
	"context"
	"errors"
	"os"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"
)

type tokenClient interface {
	VerifyIDTokenAndCheckRevoked(context.Context, string) (*firebaseauth.Token, error)
}

type FirebaseVerifier struct{ client tokenClient }

// NewFirebaseVerifier uses Application Default Credentials. Emulator mode is
// rejected because it accepts unsigned tokens and bypasses production checks.
func NewFirebaseVerifier(ctx context.Context, projectID string) (*FirebaseVerifier, error) {
	if projectID == "" {
		return nil, errors.New("FIREBASE_PROJECT_ID is required")
	}
	if os.Getenv("FIREBASE_AUTH_EMULATOR_HOST") != "" {
		return nil, errors.New("FIREBASE_AUTH_EMULATOR_HOST is unsupported; real token verification is required")
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, errors.New("initialize Firebase: check project ID and Application Default Credentials")
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, errors.New("initialize Firebase Auth: check Application Default Credentials")
	}
	return &FirebaseVerifier{client: client}, nil
}

func (v *FirebaseVerifier) Verify(ctx context.Context, raw string) (string, error) {
	token, err := v.client.VerifyIDTokenAndCheckRevoked(ctx, raw)
	if err != nil || token == nil || token.UID == "" {
		return "", errors.New("Firebase token verification failed")
	}
	return token.UID, nil
}
