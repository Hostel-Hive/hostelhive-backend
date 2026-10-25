package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"github.com/google/uuid"
)

type CheckoutRepository interface {
	Checkout(context.Context, string, string, dto.CheckoutInput, []byte) (d.CheckoutResult, error)
}
type Checkouts struct{ repo CheckoutRepository }

func NewCheckouts(repo CheckoutRepository) *Checkouts { return &Checkouts{repo} }
func (s *Checkouts) Checkout(ctx context.Context, actor, id string, v dto.CheckoutInput) (d.CheckoutResult, error) {
	id = strings.ToLower(id)
	v.RequestID = strings.ToLower(v.RequestID)
	target, te := uuid.Parse(id)
	request, re := uuid.Parse(v.RequestID)
	if te != nil || re != nil || target == uuid.Nil || request == uuid.Nil || id != target.String() || v.RequestID != request.String() || !v.Confirmed || v.DeactivateAccount == nil {
		return d.CheckoutResult{}, d.ErrInvalid
	}
	v.Reason = strings.ToLower(strings.TrimSpace(v.Reason))
	v.Notes = strings.TrimSpace(v.Notes)
	switch v.Reason {
	case "graduated", "withdrawn", "moved_out", "accommodation_cancelled", "other":
	default:
		return d.CheckoutResult{}, d.ErrInvalid
	}
	if v.Notes != "" && !validation.Text(v.Notes, 1000) {
		return d.CheckoutResult{}, d.ErrInvalid
	}
	raw, _ := json.Marshal(struct {
		StudentID string            `json:"student_id"`
		Input     dto.CheckoutInput `json:"input"`
	}{id, v})
	hash := sha256.Sum256(raw)
	return s.repo.Checkout(ctx, actor, id, v, hash[:])
}
