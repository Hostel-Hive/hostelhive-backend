package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	"github.com/google/uuid"
)

type Scans struct {
	store    ScanRepository
	cooldown time.Duration
}

func NewScans(store ScanRepository, cooldown time.Duration) *Scans { return &Scans{store, cooldown} }

func canonicalID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil && u.String() == id
}

func (s *Scans) SetDirection(ctx context.Context, actor, id string, in dto.SetDirection) (domain.Device, error) {
	if !canonicalID(id) || (in.Direction != "IN" && in.Direction != "OUT") {
		return domain.Device{}, domain.ErrInvalid
	}
	return s.store.SetDirection(ctx, actor, id, in.Direction)
}

func (s *Scans) Record(ctx context.Context, device domain.Device, in dto.Scan) (domain.Event, error) {
	if !canonicalID(in.EventID) || !strings.HasPrefix(in.QR, "HH1:") {
		return domain.Event{}, domain.ErrInvalid
	}
	token := strings.TrimPrefix(in.QR, "HH1:")
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 || strings.ToLower(token) != token {
		return domain.Event{}, domain.ErrInvalid
	}
	hash := sha256.Sum256([]byte(in.QR))
	return s.store.Record(ctx, device, in.EventID, token, hash[:], s.cooldown)
}

func (s *Scans) Presence(ctx context.Context, actor, id string) (domain.Presence, error) {
	if !canonicalID(id) {
		return domain.Presence{}, domain.ErrInvalid
	}
	return s.store.Presence(ctx, actor, id)
}

func (s *Scans) Events(ctx context.Context, actor, id string, limit, offset int) (domain.EventPage, error) {
	if !canonicalID(id) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return domain.EventPage{}, domain.ErrInvalid
	}
	return s.store.Events(ctx, actor, id, limit, offset)
}
