// Package provisioning creates Firebase and local accounts with durable retries.
package provisioning

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
)

var (
	ErrInvalidInput = errors.New("invalid provisioning input")
	ErrConflict     = errors.New("provisioning conflict")
	ErrUnavailable  = errors.New("provisioning unavailable; retry with the same request key")
	ErrInProgress   = errors.New("provisioning request in progress")
)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type Input struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

// Reservation contains no credential material.
type Reservation struct {
	Key, RequesterUID, FirebaseUID, Email, Role string
	Completed                                   bool
}
type Result struct {
	Account  authentication.Account `json:"account"`
	Replayed bool                   `json:"replayed"`
}

type Identities interface {
	EnsureDisabled(context.Context, string, string, string) error
	Enable(context.Context, string, string) error
}
type Repository interface {
	Reserve(context.Context, Reservation) error
	Begin(context.Context, string) (Work, error)
}
type Work interface {
	Reservation() Reservation
	Account(context.Context) (authentication.Account, error)
	InsertInactive(context.Context) (authentication.Account, error)
	Complete(context.Context, string) error
	Close()
}

type Service struct {
	repository Repository
	identities Identities
}

func NewService(repository Repository, identities Identities) *Service {
	return &Service{repository: repository, identities: identities}
}

func Normalize(input Input) (Input, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	parsed, err := mail.ParseAddress(input.Email)
	if err != nil || parsed.Address != input.Email || strings.Count(input.Email, "@") != 1 || len(input.Email) > 254 || strings.ContainsAny(input.Email, " \t\r\n") {
		return Input{}, ErrInvalidInput
	}
	switch input.Role {
	case authentication.RoleAdmin, authentication.RoleWarden, authentication.RoleSubWarden, authentication.RoleSecurityStaff, authentication.RoleStudent:
	default:
		return Input{}, ErrInvalidInput
	}
	length := utf8.RuneCountInString(input.Password)
	if !utf8.ValidString(input.Password) || length < 12 || length > 128 {
		return Input{}, ErrInvalidInput
	}
	return input, nil
}

func (s *Service) Provision(ctx context.Context, key string, input Input, requesterUID string) (Result, error) {
	input, err := Normalize(input)
	if err != nil || !keyPattern.MatchString(key) || strings.TrimSpace(requesterUID) == "" {
		return Result{}, ErrInvalidInput
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return Result{}, ErrUnavailable
	}
	reservation := Reservation{Key: key, RequesterUID: requesterUID, FirebaseUID: "hh_" + hex.EncodeToString(random[:]), Email: input.Email, Role: input.Role}
	// Commit the identity reservation before any external side effect. A retry
	// receives that same UID even after a timeout, crash or lost response.
	if err = s.repository.Reserve(ctx, reservation); err != nil {
		return Result{}, err
	}
	work, err := s.repository.Begin(ctx, key)
	if err != nil {
		return Result{}, err
	}
	defer work.Close()
	reservation = work.Reservation()
	if reservation.Completed {
		account, err := work.Account(ctx)
		if err != nil {
			return Result{}, err
		}
		// Replays never reset passwords, reactivate users or overwrite current roles.
		return Result{Account: account, Replayed: true}, nil
	}
	account, err := work.InsertInactive(ctx)
	if err != nil {
		return Result{}, err
	}
	if err = s.identities.EnsureDisabled(ctx, reservation.FirebaseUID, reservation.Email, input.Password); err != nil {
		return Result{}, err
	}
	if err = s.identities.Enable(ctx, reservation.FirebaseUID, reservation.Email); err != nil {
		return Result{}, err
	}
	// Activation and completion metadata commit atomically after Firebase ACKs.
	if err = work.Complete(ctx, account.UserID); err != nil {
		return Result{}, err
	}
	account.IsActive = true
	return Result{Account: account}, nil
}
