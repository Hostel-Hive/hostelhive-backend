package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"strings"
	"testing"
)

type pushStoreFake struct {
	calls int
	id    string
}

func (f *pushStoreFake) RegisterPushDevice(_ context.Context, _ string, id string, v dto.PushRegistration) (domain.PushDevice, error) {
	f.calls++
	f.id = id
	return domain.PushDevice{DeviceID: id, Platform: v.Platform, Active: true}, nil
}
func (f *pushStoreFake) ListPushDevices(context.Context, string) (domain.PushDevicePage, error) {
	f.calls++
	return domain.PushDevicePage{}, nil
}
func (f *pushStoreFake) RemovePushDevice(_ context.Context, _ string, id string) error {
	f.calls++
	f.id = id
	return nil
}
func TestPushRegistrationValidation(t *testing.T) {
	f := &pushStoreFake{}
	s := NewPush(f)
	id := "ABCDEF12-1234-4123-8123-123456789ABC"
	valid := dto.PushRegistration{Platform: "android", Token: strings.Repeat("A", 50)}
	for _, bad := range []dto.PushRegistration{{Platform: "web", Token: valid.Token}, {Platform: "android", Token: "short"}, {Platform: "ios", Token: strings.Repeat("A", 4097)}, {Platform: "ios", Token: "token with spaces invalid"}, {Platform: "ios", Token: strings.Repeat("A", 30) + "\n"}} {
		if _, err := s.RegisterPushDevice(context.Background(), "uid", id, bad); err != domain.ErrInvalid {
			t.Fatal(bad.Platform, err)
		}
	}
	for _, badID := range []string{"invalid", "00000000-0000-0000-0000-000000000000"} {
		if _, err := s.RegisterPushDevice(context.Background(), "uid", badID, valid); err != domain.ErrInvalid {
			t.Fatal(err)
		}
		if err := s.RemovePushDevice(context.Background(), "uid", badID); err != domain.ErrInvalid {
			t.Fatal(err)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid registration reached repository")
	}
	if _, err := s.RegisterPushDevice(context.Background(), "uid", id, valid); err != nil || f.id != strings.ToLower(id) {
		t.Fatal(f, err)
	}
	if err := s.RemovePushDevice(context.Background(), "uid", id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListPushDevices(context.Background(), "uid"); err != nil {
		t.Fatal(err)
	}
}
