package provisioning

import (
	"context"
	"strings"

	firebaseauth "firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/errorutils"
)

type FirebaseClient interface {
	GetUser(context.Context, string) (*firebaseauth.UserRecord, error)
	CreateUser(context.Context, *firebaseauth.UserToCreate) (*firebaseauth.UserRecord, error)
	UpdateUser(context.Context, string, *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error)
}
type FirebaseIdentities struct{ client FirebaseClient }

func NewFirebaseIdentities(client FirebaseClient) *FirebaseIdentities {
	return &FirebaseIdentities{client: client}
}

func (f *FirebaseIdentities) EnsureDisabled(ctx context.Context, uid, email, password string) error {
	user, err := f.client.GetUser(ctx, uid)
	if firebaseauth.IsUserNotFound(err) {
		user, err = f.client.CreateUser(ctx, (&firebaseauth.UserToCreate{}).UID(uid).Email(email).Password(password).Disabled(true))
		if firebaseauth.IsUIDAlreadyExists(err) {
			user, err = f.client.GetUser(ctx, uid)
		}
	}
	if firebaseauth.IsEmailAlreadyExists(err) {
		return ErrConflict
	}
	if errorutils.IsInvalidArgument(err) {
		return ErrInvalidInput
	}
	if err != nil {
		return ErrUnavailable
	}
	if !matches(user, uid, email) {
		return ErrConflict
	}
	// A prior attempt may already have enabled this reserved identity. Do not
	// reset its password or touch an unrelated account found by email.
	return nil
}
func (f *FirebaseIdentities) Enable(ctx context.Context, uid, email string) error {
	user, err := f.client.UpdateUser(ctx, uid, (&firebaseauth.UserToUpdate{}).Disabled(false))
	if err != nil {
		return ErrUnavailable
	}
	if !matches(user, uid, email) || user.Disabled {
		return ErrConflict
	}
	return nil
}
func matches(user *firebaseauth.UserRecord, uid, email string) bool {
	return user != nil && user.UserInfo != nil && user.UID == uid && strings.EqualFold(user.Email, email)
}
