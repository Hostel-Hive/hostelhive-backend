package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net"
	"net/mail"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
	student "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"github.com/google/uuid"
)

type InvitationProvider interface {
	Prepare(context.Context, string, string) (float64, error)
	Send(context.Context, string, string, string) error
	VerifySetup(context.Context, string, string, float64, string) error
}
type InvitationRepository interface {
	RegisterInvitation(context.Context, string, string, dto.StudentInvitationInput, []byte) (domain.StudentInvitation, bool, error)
	GetInvitation(context.Context, string, string) (domain.StudentInvitation, error)
	ListInvitations(context.Context, string, int, int) (domain.InvitationPage, error)
	ResendInvitation(context.Context, string, string, string, string) (domain.StudentInvitation, error)
	CompleteSetup(context.Context, string, func(context.Context, string, string, float64) error) (domain.SetupResult, error)
	DispatchInvitation(context.Context, InvitationProvider) (bool, error)
}
type Invitations struct {
	repo     InvitationRepository
	provider InvitationProvider
	domains  map[string]bool
	enabled  bool
}

func NewInvitations(repo InvitationRepository, provider InvitationProvider, domains []string, enabled bool) *Invitations {
	allowed := map[string]bool{}
	for _, d := range domains {
		allowed[strings.ToLower(d)] = true
	}
	return &Invitations{repo, provider, allowed, enabled}
}
func invitationUUID(s string) bool {
	id, e := uuid.Parse(s)
	return e == nil && id != uuid.Nil && id.String() == s
}
func (s *Invitations) normalize(v dto.StudentInvitationInput) (dto.StudentInvitationInput, error) {
	v.Email = strings.ToLower(strings.TrimSpace(v.Email))
	email, e := mail.ParseAddress(v.Email)
	parts := strings.Split(v.Email, "@")
	if !invitationUUID(v.RequestID) || e != nil || email.Address != v.Email || len(v.Email) > 254 || len(parts) != 2 || !s.domains[parts[1]] {
		return v, domain.ErrInvitationInvalid
	}
	for _, r := range v.Email {
		if r < 33 || r > 126 {
			return v, domain.ErrInvitationInvalid
		}
	}
	details, e := student.Normalize(v.Details)
	if e != nil {
		return v, domain.ErrInvitationInvalid
	}
	v.Details = details
	return v, nil
}
func (s *Invitations) Register(ctx context.Context, actor, ip string, v dto.StudentInvitationInput) (domain.StudentInvitation, bool, error) {
	if !s.enabled {
		return domain.StudentInvitation{}, false, domain.ErrInvitationUnavailable
	}
	if actor == "" {
		return domain.StudentInvitation{}, false, domain.ErrInvitationForbidden
	}
	if net.ParseIP(ip) == nil {
		return domain.StudentInvitation{}, false, domain.ErrInvitationInvalid
	}
	v, e := s.normalize(v)
	if e != nil {
		return domain.StudentInvitation{}, false, e
	}
	raw, _ := json.Marshal(v)
	hash := sha256.Sum256(raw)
	return s.repo.RegisterInvitation(ctx, actor, ip, v, hash[:])
}
func (s *Invitations) Get(ctx context.Context, actor, id string) (domain.StudentInvitation, error) {
	if !invitationUUID(id) {
		return domain.StudentInvitation{}, domain.ErrInvitationInvalid
	}
	return s.repo.GetInvitation(ctx, actor, id)
}
func (s *Invitations) List(ctx context.Context, actor string, limit, offset int) (domain.InvitationPage, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return domain.InvitationPage{}, domain.ErrInvitationInvalid
	}
	return s.repo.ListInvitations(ctx, actor, limit, offset)
}
func (s *Invitations) Resend(ctx context.Context, actor, id, request, ip string) (domain.StudentInvitation, error) {
	if !s.enabled {
		return domain.StudentInvitation{}, domain.ErrInvitationUnavailable
	}
	if !invitationUUID(id) || !invitationUUID(request) || net.ParseIP(ip) == nil {
		return domain.StudentInvitation{}, domain.ErrInvitationInvalid
	}
	return s.repo.ResendInvitation(ctx, actor, id, request, ip)
}
func (s *Invitations) Complete(ctx context.Context, actor, token string) (domain.SetupResult, error) {
	if !s.enabled || s.provider == nil {
		return domain.SetupResult{}, domain.ErrInvitationUnavailable
	}
	return s.repo.CompleteSetup(ctx, actor, func(ctx context.Context, uid, email string, baseline float64) error {
		return s.provider.VerifySetup(ctx, uid, email, baseline, token)
	})
}
func (s *Invitations) Dispatch(ctx context.Context) (bool, error) {
	if !s.enabled || s.provider == nil {
		return false, nil
	}
	return s.repo.DispatchInvitation(ctx, s.provider)
}
