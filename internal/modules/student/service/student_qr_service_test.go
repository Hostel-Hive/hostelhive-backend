package service_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"image/png"
	"strings"
	"testing"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

type qrStore struct {
	token, actor, id, candidate string
	calls                       int
	err                         error
}

func (s *qrStore) QR(_ context.Context, actor, id, candidate string) (string, error) {
	s.calls++
	s.actor, s.id, s.candidate = actor, id, candidate
	return s.token, s.err
}

func TestQRScannableAndPrivate(t *testing.T) {
	fake := &qrStore{token: strings.Repeat("ab", 32)}
	s := svc.NewQRs(fake)
	id := "ABCDEFAB-1234-4567-89AB-ABCDEFABCDEF"
	first, err := s.PNG(context.Background(), "verified-user", id)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.PNG(context.Background(), "verified-user", id)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("QR changed on retrieval")
	}
	if fake.actor != "verified-user" || fake.id != strings.ToLower(id) {
		t.Fatal("identity was not preserved")
	}
	if random, err := hex.DecodeString(fake.candidate); err != nil || len(random) != 32 {
		t.Fatal("invalid random candidate")
	}
	img, err := png.Decode(bytes.NewReader(first))
	if err != nil || img.Bounds().Dx() != 320 || img.Bounds().Dy() != 320 {
		t.Fatal("invalid PNG dimensions")
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := qrcode.NewQRCodeReader().Decode(bitmap, nil)
	if err != nil || decoded.GetText() != svc.QRPrefix+fake.token {
		t.Fatalf("QR decoding failed: %v", err)
	}
	if strings.Contains(decoded.GetText(), "verified-user") || strings.Contains(decoded.GetText(), strings.ToLower(id)) {
		t.Fatal("personal identity in payload")
	}
	if _, err = s.PNG(context.Background(), "verified-user", ""); err != nil || fake.id != "" {
		t.Fatal("self lookup failed")
	}
}

func TestQRValidationAndFailures(t *testing.T) {
	for _, input := range []struct{ actor, id string }{{"", ""}, {" ", ""}, {"uid", "invalid"}} {
		fake := &qrStore{}
		if _, err := svc.NewQRs(fake).PNG(context.Background(), input.actor, input.id); !errors.Is(err, d.ErrInvalid) || fake.calls != 0 {
			t.Fatal("invalid input reached persistence")
		}
	}
	for _, token := range []string{"", "invalid", strings.Repeat("AB", 32), strings.Repeat("ab", 31)} {
		if _, err := svc.NewQRs(&qrStore{token: token}).PNG(context.Background(), "uid", ""); !errors.Is(err, d.ErrUnavailable) {
			t.Fatal("corrupt persisted token accepted")
		}
	}
	for _, failure := range []error{d.ErrNotFound, d.ErrForbidden, d.ErrUnavailable} {
		if _, err := svc.NewQRs(&qrStore{err: failure}).PNG(context.Background(), "uid", ""); !errors.Is(err, failure) {
			t.Fatal("repository error lost")
		}
	}
}
