package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"github.com/google/uuid"
)

type Devices struct{ repo DeviceRepository }

func NewDevices(repo DeviceRepository) *Devices { return &Devices{repo: repo} }

// A 256-bit random secret permits a one-way SHA-256 verifier; this is not a
// human-chosen password. Device ID is a selector, not authentication evidence.
func newCredential(id string) (string, []byte, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", nil, domain.ErrUnavailable
	}
	credential := "hhd1." + id + "." + base64.RawURLEncoding.EncodeToString(secret[:])
	hash := sha256.Sum256([]byte(credential))
	return credential, hash[:], nil
}

func (s *Devices) Register(ctx context.Context, actor string, in dto.RegisterDevice) (dto.IssuedCredential, error) {
	in.Name = strings.TrimSpace(in.Name)
	if strings.TrimSpace(actor) == "" || !validation.Text(in.Name, 120) {
		return dto.IssuedCredential{}, domain.ErrInvalid
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return dto.IssuedCredential{}, domain.ErrUnavailable
	}
	credential, hash, err := newCredential(id.String())
	if err != nil {
		return dto.IssuedCredential{}, err
	}
	device, err := s.repo.Register(ctx, actor, id.String(), in.Name, hash)
	if err != nil {
		return dto.IssuedCredential{}, err
	}
	return dto.IssuedCredential{Device: device, Credential: credential}, nil
}

func (s *Devices) List(ctx context.Context, actor string, limit, offset int) (domain.DevicePage, error) {
	if strings.TrimSpace(actor) == "" || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return domain.DevicePage{}, domain.ErrInvalid
	}
	return s.repo.List(ctx, actor, limit, offset)
}

func (s *Devices) Rotate(ctx context.Context, actor, id string) (dto.IssuedCredential, error) {
	if strings.TrimSpace(actor) == "" || !validation.UUID(id) {
		return dto.IssuedCredential{}, domain.ErrInvalid
	}
	id = strings.ToLower(id)
	credential, hash, err := newCredential(id)
	if err != nil {
		return dto.IssuedCredential{}, err
	}
	device, err := s.repo.Rotate(ctx, actor, id, hash)
	if err != nil {
		return dto.IssuedCredential{}, err
	}
	return dto.IssuedCredential{Device: device, Credential: credential}, nil
}

func (s *Devices) Deactivate(ctx context.Context, actor, id string) (domain.Device, error) {
	if strings.TrimSpace(actor) == "" || !validation.UUID(id) {
		return domain.Device{}, domain.ErrInvalid
	}
	return s.repo.Deactivate(ctx, actor, strings.ToLower(id))
}

func (s *Devices) Authenticate(ctx context.Context, credential string) (domain.Device, error) {
	parts := strings.Split(credential, ".")
	if len(parts) != 3 || parts[0] != "hhd1" || !validation.UUID(parts[1]) || parts[1] != strings.ToLower(parts[1]) || len(parts[2]) != 43 {
		return domain.Device{}, domain.ErrUnauthorized
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(secret) != 32 || base64.RawURLEncoding.EncodeToString(secret) != parts[2] {
		return domain.Device{}, domain.ErrUnauthorized
	}
	material, err := s.repo.Authentication(ctx, parts[1])
	if err != nil {
		return domain.Device{}, err
	}
	hash := sha256.Sum256([]byte(credential))
	if !material.Device.IsActive || material.Device.DeviceID != parts[1] || len(material.Hash) != 32 || subtle.ConstantTimeCompare(hash[:], material.Hash) != 1 {
		return domain.Device{}, domain.ErrUnauthorized
	}
	return material.Device, nil
}
