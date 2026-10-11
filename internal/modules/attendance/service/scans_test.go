package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	"github.com/google/uuid"
)

type fakeScans struct {
	calls    int
	token    string
	hash     []byte
	cooldown time.Duration
}

func (f *fakeScans) SetDirection(context.Context, string, string, string) (d.Device, error) {
	f.calls++
	return d.Device{}, d.ErrUnavailable
}
func (f *fakeScans) Record(_ context.Context, _ d.Device, _ string, token string, hash []byte, cooldown time.Duration) (d.Event, error) {
	f.calls++
	f.token = token
	f.hash = hash
	f.cooldown = cooldown
	return d.Event{}, d.ErrUnavailable
}
func (f *fakeScans) Presence(context.Context, string, string) (d.Presence, error) {
	f.calls++
	return d.Presence{}, d.ErrUnavailable
}
func (f *fakeScans) Events(context.Context, string, string, int, int) (d.EventPage, error) {
	f.calls++
	return d.EventPage{}, d.ErrUnavailable
}

func TestScanValidationBeforePersistence(t *testing.T) {
	ctx := context.Background()
	f := &fakeScans{}
	s := svc.NewScans(f, 10*time.Second)
	id := uuid.NewString()
	qr := "HH1:" + strings.Repeat("ab", 32)
	for _, in := range []dto.Scan{{EventID: "bad", QR: qr}, {EventID: uuid.Nil.String(), QR: qr}, {EventID: id, QR: "bad"}, {EventID: id, QR: "HH1:" + strings.Repeat("AB", 32)}, {EventID: id, QR: "HH1:abc"}, {EventID: id, QR: qr + " "}} {
		if _, err := s.Record(ctx, d.Device{}, in); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("malformed scan accepted", err)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid scan reached persistence")
	}
	if _, err := s.Record(ctx, d.Device{}, dto.Scan{EventID: id, QR: qr}); !errors.Is(err, d.ErrUnavailable) || f.token != strings.TrimPrefix(qr, "HH1:") || len(f.hash) != 32 || f.cooldown != 10*time.Second {
		t.Fatal("scan contract or error lost")
	}
	for _, in := range []dto.SetDirection{{Direction: "in"}, {Direction: ""}, {Direction: "SIDE"}} {
		if _, err := s.SetDirection(ctx, "admin", id, in); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("direction accepted")
		}
	}
	if _, err := s.SetDirection(ctx, "admin", "bad", dto.SetDirection{Direction: "IN"}); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("bad device ID")
	}
	if _, err := s.SetDirection(ctx, "admin", id, dto.SetDirection{Direction: "OUT"}); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("direction error lost")
	}
	if _, err := s.Presence(ctx, "admin", "bad"); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("bad student ID")
	}
	if _, err := s.Presence(ctx, "admin", id); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("presence error lost")
	}
	for _, bounds := range [][2]int{{0, 0}, {101, 0}, {1, -1}, {1, 1000001}} {
		if _, err := s.Events(ctx, "admin", id, bounds[0], bounds[1]); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid paging")
		}
	}
	if _, err := s.Events(ctx, "admin", id, 1, 0); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("history error lost")
	}
}
