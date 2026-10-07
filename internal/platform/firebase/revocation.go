package firebase

import (
	"context"

	firebaseauth "firebase.google.com/go/v4/auth"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
)

type RevocationClient interface {
	UpdateUser(context.Context, string, *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error)
	RevokeRefreshTokens(context.Context, string) error
}

type FirebaseRevoker struct{ client RevocationClient }

func NewFirebaseRevoker(client RevocationClient) *FirebaseRevoker {
	return &FirebaseRevoker{client: client}
}

func (r *FirebaseRevoker) DisableAndRevoke(ctx context.Context, uid string) error {
	user, err := r.client.UpdateUser(ctx, uid, (&firebaseauth.UserToUpdate{}).Disabled(true))
	if firebaseauth.IsUserNotFound(err) {
		return nil
	}
	if err != nil || user == nil || user.UserInfo == nil || user.UID != uid || !user.Disabled {
		return domain.ErrManagementUnavailable
	}
	err = r.client.RevokeRefreshTokens(ctx, uid)
	if err != nil && !firebaseauth.IsUserNotFound(err) {
		return domain.ErrManagementUnavailable
	}
	return nil
}
