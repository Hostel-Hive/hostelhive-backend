package middleware

import (
	"context"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
)

type verifyFunc func(context.Context, string) (string, error)

func (f verifyFunc) Verify(ctx context.Context, token string) (string, error) { return f(ctx, token) }

type accountFunc func(context.Context, string) (domain.Account, error)

func (f accountFunc) FindByFirebaseUID(ctx context.Context, uid string) (domain.Account, error) {
	return f(ctx, uid)
}
