package firebase

import (
	"context"
	"errors"
	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"reflect"
	"testing"
)

type activationClientFake struct {
	steps                        []string
	lookup, update               *firebaseauth.UserRecord
	getErr, revokeErr, updateErr error
}

func (f *activationClientFake) GetUser(context.Context, string) (*firebaseauth.UserRecord, error) {
	f.steps = append(f.steps, "lookup")
	return f.lookup, f.getErr
}
func (f *activationClientFake) RevokeRefreshTokens(context.Context, string) error {
	f.steps = append(f.steps, "revoke")
	return f.revokeErr
}
func (f *activationClientFake) UpdateUser(context.Context, string, *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error) {
	f.steps = append(f.steps, "enable")
	return f.update, f.updateErr
}
func TestReactivationIdentityAndAcknowledgment(t *testing.T) {
	user := func(disabled bool) *firebaseauth.UserRecord {
		return &firebaseauth.UserRecord{UserInfo: &firebaseauth.UserInfo{UID: "uid", Email: "user@example.invalid"}, Disabled: disabled}
	}
	for _, tc := range []struct {
		name                   string
		lookup, update         *firebaseauth.UserRecord
		get, revoke, updateErr error
		want                   error
		steps                  []string
	}{
		{"success", user(true), user(false), nil, nil, nil, nil, []string{"lookup", "revoke", "enable"}},
		{"wrong identity", nil, user(false), nil, nil, nil, domain.ErrIdentityMismatch, []string{"lookup"}},
		{"lookup fails", nil, nil, errors.New("private provider failure"), nil, nil, domain.ErrManagementUnavailable, []string{"lookup"}},
		{"revocation fails", user(true), user(false), nil, errors.New("failed"), nil, domain.ErrManagementUnavailable, []string{"lookup", "revoke"}},
		{"enable fails", user(true), nil, nil, nil, errors.New("failed"), domain.ErrManagementUnavailable, []string{"lookup", "revoke", "enable"}},
		{"still disabled", user(true), user(true), nil, nil, nil, domain.ErrManagementUnavailable, []string{"lookup", "revoke", "enable"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &activationClientFake{lookup: tc.lookup, update: tc.update, getErr: tc.get, revokeErr: tc.revoke, updateErr: tc.updateErr}
			err := NewFirebaseReactivator(f).Reactivate(context.Background(), "uid", "USER@example.invalid")
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(f.steps, tc.steps) {
				t.Fatalf("%v %v", err, f.steps)
			}
		})
	}
}
