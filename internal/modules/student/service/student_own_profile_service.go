package service

import (
	"context"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
)

type OwnProfileRepository interface {
	OwnProfile(context.Context, string) (d.OwnProfile, error)
}
type OwnProfiles struct{ repo OwnProfileRepository }

func NewOwnProfiles(repo OwnProfileRepository) *OwnProfiles { return &OwnProfiles{repo: repo} }
func (s *OwnProfiles) Get(ctx context.Context, actor string) (d.OwnProfile, error) {
	if actor == "" {
		return d.OwnProfile{}, d.ErrForbidden
	}
	return s.repo.OwnProfile(ctx, actor)
}
