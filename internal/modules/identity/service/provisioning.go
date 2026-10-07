package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/dto"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type ProvisioningService struct {
	repository Repository
	identities Identities
}

func NewProvisioningService(repository Repository, identities Identities) *ProvisioningService {
	return &ProvisioningService{repository: repository, identities: identities}
}

func NormalizeProvisioning(input dto.Input) (dto.Input, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	parsed, err := mail.ParseAddress(input.Email)
	if err != nil || parsed.Address != input.Email || strings.Count(input.Email, "@") != 1 || len(input.Email) > 254 || strings.ContainsAny(input.Email, " \t\r\n") {
		return dto.Input{}, domain.ErrProvisionInvalidInput
	}
	switch input.Role {
	case domain.RoleAdmin, domain.RoleWarden, domain.RoleSubWarden, domain.RoleSecurityStaff, domain.RoleStudent:
	default:
		return dto.Input{}, domain.ErrProvisionInvalidInput
	}
	length := utf8.RuneCountInString(input.Password)
	if !utf8.ValidString(input.Password) || length < 12 || length > 128 {
		return dto.Input{}, domain.ErrProvisionInvalidInput
	}
	return input, nil
}

func (s *ProvisioningService) Provision(ctx context.Context, key string, input dto.Input, requesterUID string) (dto.Result, error) {
	input, err := NormalizeProvisioning(input)
	if err != nil || !keyPattern.MatchString(key) || strings.TrimSpace(requesterUID) == "" {
		return dto.Result{}, domain.ErrProvisionInvalidInput
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return dto.Result{}, domain.ErrProvisionUnavailable
	}
	reservation := domain.Reservation{Key: key, RequesterUID: requesterUID, FirebaseUID: "hh_" + hex.EncodeToString(random[:]), Email: input.Email, Role: input.Role}
	// Commit the identity reservation before any external side effect. A retry
	// receives that same UID even after a timeout, crash or lost response.
	if err = s.repository.Reserve(ctx, reservation); err != nil {
		return dto.Result{}, err
	}
	work, err := s.repository.Begin(ctx, key)
	if err != nil {
		return dto.Result{}, err
	}
	defer work.Close()
	reservation = work.Reservation()
	if reservation.Completed {
		account, err := work.Account(ctx)
		if err != nil {
			return dto.Result{}, err
		}
		// Replays never reset passwords, reactivate users or overwrite current roles.
		return dto.Result{Account: account, Replayed: true}, nil
	}
	account, err := work.InsertInactive(ctx)
	if err != nil {
		return dto.Result{}, err
	}
	if err = s.identities.EnsureDisabled(ctx, reservation.FirebaseUID, reservation.Email, input.Password); err != nil {
		return dto.Result{}, err
	}
	if err = s.identities.Enable(ctx, reservation.FirebaseUID, reservation.Email); err != nil {
		return dto.Result{}, err
	}
	// Activation and completion metadata commit atomically after Firebase ACKs.
	if err = work.Complete(ctx, account.UserID); err != nil {
		return dto.Result{}, err
	}
	account.IsActive = true
	return dto.Result{Account: account}, nil
}
