package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	qrcode "github.com/skip2/go-qrcode"
)

const QRPrefix = "HH1:"

type QRs struct{ store QRRepository }

func NewQRs(store QRRepository) *QRs { return &QRs{store: store} }

// PNG returns a stable QR, issued lazily on first authorized retrieval.
// The payload is a versioned opaque identifier, not an authentication token.
func (s *QRs) PNG(ctx context.Context, actor, studentID string) ([]byte, error) {
	if strings.TrimSpace(actor) == "" || (studentID != "" && !validation.UUID(studentID)) {
		return nil, sdomain.ErrInvalid
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, sdomain.ErrUnavailable
	}
	token, err := s.store.QR(ctx, actor, strings.ToLower(studentID), hex.EncodeToString(random[:]))
	if err != nil {
		return nil, err
	}
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 || token != strings.ToLower(token) {
		return nil, sdomain.ErrUnavailable
	}
	image, err := qrcode.Encode(QRPrefix+token, qrcode.Medium, 320)
	if err != nil {
		return nil, sdomain.ErrUnavailable
	}
	return image, nil
}
