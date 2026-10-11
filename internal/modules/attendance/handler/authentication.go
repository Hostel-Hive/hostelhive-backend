package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/response"
)

type deviceKey struct{}

func DeviceFromContext(ctx context.Context) (domain.Device, bool) {
	d, ok := ctx.Value(deviceKey{}).(domain.Device)
	return d, ok
}

// DeviceMiddleware authenticates only device credentials. It never interprets
// them as Firebase identities or grants access to user management endpoints.
func (a *API) DeviceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			response.Error(w, 401, "unauthorized")
			return
		}
		fields := strings.Fields(values[0])
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			response.Error(w, 401, "unauthorized")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), a.authTimeout)
		device, err := a.service.Authenticate(ctx, fields[1])
		cancel()
		if err != nil {
			failure(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), deviceKey{}, device)))
	})
}
