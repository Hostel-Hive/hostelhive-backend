package accountmanagement

import (
	"context"

	firebaseauth "firebase.google.com/go/v4/auth"
)

type FirebaseClient interface {
	UpdateUser(context.Context, string, *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error)
	RevokeRefreshTokens(context.Context, string) error
}
type FirebaseRevoker struct{ client FirebaseClient }

func NewFirebaseRevoker(client FirebaseClient) *FirebaseRevoker {
	return &FirebaseRevoker{client: client}
}
func (r *FirebaseRevoker) DisableAndRevoke(ctx context.Context, uid string) error {
	user, err := r.client.UpdateUser(ctx, uid, (&firebaseauth.UserToUpdate{}).Disabled(true))
	if firebaseauth.IsUserNotFound(err) {
		return nil
	}
	if err != nil || user == nil || user.UserInfo == nil || user.UID != uid || !user.Disabled {
		return ErrUnavailable
	}
	err = r.client.RevokeRefreshTokens(ctx, uid)
	if err != nil && !firebaseauth.IsUserNotFound(err) {
		return ErrUnavailable
	}
	return nil
}
