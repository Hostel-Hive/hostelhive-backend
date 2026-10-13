package service_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
)

type fakeDevices struct {
	device d.Device
	hash   []byte
	err    error
	calls  int
	actor  string
}

func (f *fakeDevices) Register(_ context.Context, actor, id, name string, hash []byte) (d.Device, error) {
	f.calls++
	f.actor = actor
	if f.err != nil {
		return d.Device{}, f.err
	}
	f.device = d.Device{DeviceID: id, Name: name, IsActive: true, CredentialVersion: 1}
	f.hash = append([]byte{}, hash...)
	return f.device, nil
}
func (f *fakeDevices) List(context.Context, string, int, int) (d.DevicePage, error) {
	f.calls++
	return d.DevicePage{}, f.err
}
func (f *fakeDevices) Rotate(_ context.Context, actor, id string, hash []byte) (d.Device, error) {
	f.calls++
	f.actor = actor
	if f.err != nil {
		return d.Device{}, f.err
	}
	f.hash = append([]byte{}, hash...)
	f.device.CredentialVersion++
	return f.device, nil
}
func (f *fakeDevices) Deactivate(context.Context, string, string) (d.Device, error) {
	f.calls++
	f.device.IsActive = false
	return f.device, f.err
}
func (f *fakeDevices) Authentication(context.Context, string) (d.AuthenticationMaterial, error) {
	f.calls++
	return d.AuthenticationMaterial{Device: f.device, Hash: f.hash}, f.err
}

func TestDeviceCredentialLifecycle(t *testing.T) {
	fake := &fakeDevices{}
	s := svc.NewDevices(fake)
	issued, err := s.Register(context.Background(), "verified-admin", dto.RegisterDevice{Name: " Gate-01 "})
	if err != nil || issued.Device.Name != "Gate-01" || fake.actor != "verified-admin" {
		t.Fatal("registration failed")
	}
	if len(fake.hash) != 32 || strings.Contains(string(fake.hash), issued.Credential) {
		t.Fatal("raw credential persisted")
	}
	expected := sha256.Sum256([]byte(issued.Credential))
	if string(fake.hash) != string(expected[:]) {
		t.Fatal("credential verifier mismatch")
	}
	if d, err := s.Authenticate(context.Background(), issued.Credential); err != nil || d.DeviceID != issued.Device.DeviceID {
		t.Fatal("valid credential rejected")
	}
	parts := strings.Split(issued.Credential, ".")
	forged := parts[0] + "." + parts[1] + "." + strings.Repeat("A", 43)
	if _, err := s.Authenticate(context.Background(), forged); !errors.Is(err, d.ErrUnauthorized) {
		t.Fatal("forged credential accepted")
	}
	rotated, err := s.Rotate(context.Background(), "verified-admin", strings.ToUpper(issued.Device.DeviceID))
	if err != nil || rotated.Credential == issued.Credential || rotated.Device.CredentialVersion != 2 {
		t.Fatal("rotation failed")
	}
	if _, err := s.Authenticate(context.Background(), issued.Credential); !errors.Is(err, d.ErrUnauthorized) {
		t.Fatal("old credential accepted")
	}
	if _, err := s.Authenticate(context.Background(), rotated.Credential); err != nil {
		t.Fatal("new credential rejected")
	}
	if _, err := s.Deactivate(context.Background(), "verified-admin", issued.Device.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(context.Background(), rotated.Credential); !errors.Is(err, d.ErrUnauthorized) {
		t.Fatal("inactive device accepted")
	}
	// Rotation cannot implicitly re-enable an inactive device.
	rotated, err = s.Rotate(context.Background(), "verified-admin", issued.Device.DeviceID)
	if err != nil || rotated.Device.IsActive {
		t.Fatal("rotation reactivated scanner")
	}
	if _, err := s.Authenticate(context.Background(), rotated.Credential); !errors.Is(err, d.ErrUnauthorized) {
		t.Fatal("inactive rotated credential accepted")
	}
}

func TestDeviceInputAndFailureIsolation(t *testing.T) {
	ctx := context.Background()
	for _, credential := range []string{"", "firebase-id-token", "hhd1.bad.secret", "hhd1.11111111-1111-4111-8111-111111111111." + strings.Repeat("!", 43), "hhd1.11111111-1111-4111-8111-111111111111." + strings.Repeat("a", 42) + "/"} {
		fake := &fakeDevices{}
		if _, err := svc.NewDevices(fake).Authenticate(ctx, credential); !errors.Is(err, d.ErrUnauthorized) || fake.calls != 0 {
			t.Fatal("malformed credential reached persistence")
		}
	}
	for _, name := range []string{"", " ", "gate\nname", strings.Repeat("a", 121)} {
		fake := &fakeDevices{}
		if _, err := svc.NewDevices(fake).Register(ctx, "uid", dto.RegisterDevice{Name: name}); !errors.Is(err, d.ErrInvalid) || fake.calls != 0 {
			t.Fatal("invalid name accepted")
		}
	}
	fake := &fakeDevices{}
	s := svc.NewDevices(fake)
	if _, err := s.Register(ctx, "", dto.RegisterDevice{Name: "gate"}); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("blank actor accepted")
	}
	if _, err := s.Rotate(ctx, "uid", "bad"); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("invalid rotation ID accepted")
	}
	if _, err := s.Deactivate(ctx, "uid", "bad"); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("invalid deactivation ID accepted")
	}
	for _, paging := range [][2]int{{0, 0}, {101, 0}, {20, -1}, {20, 1000001}} {
		if _, err := s.List(ctx, "uid", paging[0], paging[1]); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid paging accepted")
		}
	}
	for _, failure := range []error{d.ErrConflict, d.ErrForbidden, d.ErrUnavailable, d.ErrNotFound} {
		fake.err = failure
		if issued, err := s.Register(ctx, "uid", dto.RegisterDevice{Name: "gate"}); !errors.Is(err, failure) || issued.Credential != "" {
			t.Fatal("failed registration exposed credential")
		}
		if issued, err := s.Rotate(ctx, "uid", "11111111-1111-4111-8111-111111111111"); !errors.Is(err, failure) || issued.Credential != "" {
			t.Fatal("failed rotation exposed credential")
		}
	}
	fake.err = nil
	if _, err := s.List(ctx, "uid", 20, 0); err != nil {
		t.Fatal(err)
	}
	issued, err := s.Register(ctx, "uid", dto.RegisterDevice{Name: "gate"})
	if err != nil {
		t.Fatal(err)
	}
	fake.err = d.ErrUnavailable
	if _, err := s.Authenticate(ctx, issued.Credential); !errors.Is(err, d.ErrUnavailable) {
		t.Fatal("database failure hidden as invalid credentials")
	}
	fake.err = nil
	fake.hash = nil
	if _, err := s.Authenticate(ctx, issued.Credential); !errors.Is(err, d.ErrUnauthorized) {
		t.Fatal("invalid verifier accepted")
	}
}
