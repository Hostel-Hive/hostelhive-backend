package service_test

import (
	"context"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"testing"
)

type ownRepo struct {
	calls int
	actor string
	err   error
}

func (r *ownRepo) OwnProfile(_ context.Context, actor string) (d.OwnProfile, error) {
	r.calls++
	r.actor = actor
	return d.OwnProfile{StudentID: "own"}, r.err
}
func TestOwnProfileIdentity(t *testing.T) {
	r := &ownRepo{}
	svc := s.NewOwnProfiles(r)
	if _, err := svc.Get(context.Background(), ""); !errors.Is(err, d.ErrForbidden) || r.calls != 0 {
		t.Fatal("empty identity accepted")
	}
	r.err = d.ErrNotFound
	if _, err := svc.Get(context.Background(), "verified"); !errors.Is(err, d.ErrNotFound) || r.actor != "verified" {
		t.Fatal("identity/error not preserved")
	}
	r.err = nil
	if p, err := svc.Get(context.Background(), "verified"); err != nil || p.StudentID != "own" {
		t.Fatal("profile not returned")
	}
}
