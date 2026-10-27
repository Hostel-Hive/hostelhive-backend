package service

import (
	"context"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"testing"
)

type ownImageProfileFake struct {
	calls int
	err   error
}

func (f *ownImageProfileFake) OwnProfile(context.Context, string) (d.OwnProfile, error) {
	f.calls++
	return d.OwnProfile{StudentID: "11111111-1111-4111-8111-111111111111"}, f.err
}
func TestOwnImageResolvesIdentityAndReusesLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, actor        string
		profileErr, putErr error
		want               error
		reserve            bool
	}{
		{name: "success", actor: "verified-uid", reserve: true},
		{name: "no actor", actor: " ", want: d.ErrForbidden},
		{name: "missing profile", actor: "uid", profileErr: d.ErrNotFound, want: d.ErrNotFound},
		{name: "inactive account", actor: "uid", profileErr: d.ErrForbidden, want: d.ErrForbidden},
		{name: "storage failure", actor: "uid", putErr: errors.New("secret"), want: d.ErrUnavailable, reserve: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profiles := &ownImageProfileFake{err: tc.profileErr}
			objects := &imageFixture{putErr: tc.putErr}
			result, err := NewOwnImages(profiles, objects, objects).Upload(context.Background(), tc.actor, "image/png", testPNG(2))
			if !errors.Is(err, tc.want) || objects.reserved != tc.reserve {
				t.Fatal(result, err, objects.reserved)
			}
			if err == nil && result.ProfileImageURL != "/api/v1/me/student-profile/image" {
				t.Fatal("unsafe download URL", result)
			}
			if tc.actor == " " && profiles.calls != 0 {
				t.Fatal("unauthenticated repository access")
			}
		})
	}
	profiles := &ownImageProfileFake{err: d.ErrNotFound}
	objects := &imageFixture{}
	service := NewOwnImages(profiles, objects, objects)
	if err := service.Remove(context.Background(), "uid"); !errors.Is(err, d.ErrNotFound) {
		t.Fatal(err)
	}
	if err := service.Remove(context.Background(), ""); !errors.Is(err, d.ErrForbidden) {
		t.Fatal(err)
	}
	profiles.err = nil
	if err := service.Remove(context.Background(), "uid"); err != nil {
		t.Fatal(err)
	}
}
