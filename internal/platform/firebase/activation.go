package firebase

import (
	"context"
	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"strings"
)

type ActivationClient interface {
	GetUser(context.Context, string) (*firebaseauth.UserRecord, error)
	UpdateUser(context.Context, string, *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error)
	RevokeRefreshTokens(context.Context, string) error
}
type FirebaseReactivator struct{ client ActivationClient }

func NewFirebaseReactivator(c ActivationClient) *FirebaseReactivator { return &FirebaseReactivator{c} }
func (r *FirebaseReactivator) Reactivate(ctx context.Context, uid, email string) error {
	u, err := r.client.GetUser(ctx, uid)
	if firebaseauth.IsUserNotFound(err) {
		return domain.ErrIdentityMissing
	}
	if err != nil {
		return domain.ErrManagementUnavailable
	}
	if !matchesIdentity(u, uid, email) {
		return domain.ErrIdentityMismatch
	}
	// Invalidate old sessions before enabling sign-in. Local access remains blocked.
	if r.client.RevokeRefreshTokens(ctx, uid) != nil {
		return domain.ErrManagementUnavailable
	}
	u, err = r.client.UpdateUser(ctx, uid, (&firebaseauth.UserToUpdate{}).Disabled(false))
	if err != nil || !matchesIdentity(u, uid, email) || u.Disabled {
		return domain.ErrManagementUnavailable
	}
	return nil
}
func matchesIdentity(u *firebaseauth.UserRecord, uid, email string) bool {
	return u != nil && u.UserInfo != nil && u.UID == uid && strings.EqualFold(u.Email, email)
}
