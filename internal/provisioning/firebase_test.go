package provisioning

import (
	"context"
	"errors"
	"testing"

	firebaseauth "firebase.google.com/go/v4/auth"
)

type fakeFirebase struct {
	get    func(context.Context, string) (*firebaseauth.UserRecord, error)
	create func(context.Context, *firebaseauth.UserToCreate) (*firebaseauth.UserRecord, error)
	update func(context.Context, string, *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error)
}

func (f fakeFirebase) GetUser(ctx context.Context, uid string) (*firebaseauth.UserRecord, error) {
	return f.get(ctx, uid)
}
func (f fakeFirebase) CreateUser(ctx context.Context, p *firebaseauth.UserToCreate) (*firebaseauth.UserRecord, error) {
	return f.create(ctx, p)
}
func (f fakeFirebase) UpdateUser(ctx context.Context, uid string, p *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error) {
	return f.update(ctx, uid, p)
}
func firebaseUser(uid, email string, disabled bool) *firebaseauth.UserRecord {
	return &firebaseauth.UserRecord{UserInfo: &firebaseauth.UserInfo{UID: uid, Email: email}, Disabled: disabled}
}
func TestFirebaseResumeDoesNotResetPassword(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		client := fakeFirebase{get: func(context.Context, string) (*firebaseauth.UserRecord, error) {
			return firebaseUser("reserved-uid", "USER@EXAMPLE.INVALID", disabled), nil
		},
			create: func(context.Context, *firebaseauth.UserToCreate) (*firebaseauth.UserRecord, error) {
				t.Fatal("retry reset identity or password")
				return nil, nil
			},
			update: func(ctx context.Context, uid string, p *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error) {
				if uid != "reserved-uid" {
					t.Fatal("wrong UID enabled")
				}
				return firebaseUser(uid, "user@example.invalid", false), nil
			}}
		identities := NewFirebaseIdentities(client)
		if err := identities.EnsureDisabled(context.Background(), "reserved-uid", "user@example.invalid", "Different-password-20"); err != nil {
			t.Fatal(err)
		}
		if err := identities.Enable(context.Background(), "reserved-uid", "user@example.invalid"); err != nil {
			t.Fatal(err)
		}
	}
}
func TestFirebaseIdentityMismatchAndSafeErrors(t *testing.T) {
	for _, user := range []*firebaseauth.UserRecord{nil, firebaseUser("other", "user@example.invalid", false), firebaseUser("reserved-uid", "other@example.invalid", false)} {
		client := fakeFirebase{get: func(context.Context, string) (*firebaseauth.UserRecord, error) { return user, nil }}
		if err := NewFirebaseIdentities(client).EnsureDisabled(context.Background(), "reserved-uid", "user@example.invalid", "password"); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	client := fakeFirebase{get: func(context.Context, string) (*firebaseauth.UserRecord, error) {
		return nil, errors.New("secret credential")
	}, update: func(context.Context, string, *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error) {
		return nil, errors.New("secret credential")
	}}
	if err := NewFirebaseIdentities(client).EnsureDisabled(context.Background(), "uid", "email", "password"); err != ErrUnavailable {
		t.Fatal(err)
	}
	if err := NewFirebaseIdentities(client).Enable(context.Background(), "uid", "email"); err != ErrUnavailable {
		t.Fatal(err)
	}
}
