package handler

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
)

type CheckoutService interface {
	Checkout(context.Context, string, string, dto.CheckoutInput) (d.CheckoutResult, error)
}
type CheckoutAPI struct {
	service CheckoutService
	timeout time.Duration
}

func NewCheckoutAPI(s CheckoutService, timeout time.Duration) *CheckoutAPI {
	return &CheckoutAPI{s, timeout}
}
func (a *CheckoutAPI) Checkout(w http.ResponseWriter, r *http.Request) {
	actor, ok := staff(w, r)
	if !ok {
		return
	}
	if actor.Role != identity.RoleAdmin {
		reject(w, 403, "forbidden")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, d.ErrInvalid)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		reject(w, 415, "unsupported_media_type")
		return
	}
	var input dto.CheckoutInput
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout)
	defer cancel()
	result, err := a.service.Checkout(ctx, actor.FirebaseUID, r.PathValue("studentID"), input)
	switch {
	case errors.Is(err, d.ErrCheckoutConflict):
		reject(w, 409, "checkout_conflict")
	case errors.Is(err, d.ErrAlreadyCheckedOut):
		reject(w, 409, "student_already_checked_out")
	case errors.Is(err, d.ErrCheckoutIneligible):
		reject(w, 409, "student_account_required")
	case err != nil:
		failure(w, err)
	default:
		reply(w, http.StatusOK, result)
	}
}
