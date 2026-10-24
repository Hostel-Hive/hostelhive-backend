package service

import (
	"context"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"strings"
)

type OwnImages struct {
	profiles OwnProfileRepository
	images   *Images
}

func NewOwnImages(profiles OwnProfileRepository, images ImageRepository, objects Objects) *OwnImages {
	return &OwnImages{profiles: profiles, images: NewImages(images, objects)}
}
func (s *OwnImages) Upload(ctx context.Context, actor, mime string, raw []byte) (d.OwnImageResult, error) {
	if strings.TrimSpace(actor) == "" {
		return d.OwnImageResult{}, d.ErrForbidden
	}
	profile, err := s.profiles.OwnProfile(ctx, actor)
	if err != nil {
		return d.OwnImageResult{}, err
	}
	uploaded, err := s.images.Upload(ctx, actor, profile.StudentID, mime, raw)
	if err != nil {
		return d.OwnImageResult{}, err
	}
	return d.OwnImageResult{StudentID: uploaded.StudentID, ProfileImageURL: "/api/v1/me/student-profile/image"}, nil
}
func (s *OwnImages) Remove(ctx context.Context, actor string) error {
	if strings.TrimSpace(actor) == "" {
		return d.ErrForbidden
	}
	profile, err := s.profiles.OwnProfile(ctx, actor)
	if err != nil {
		return err
	}
	return s.images.Remove(ctx, actor, profile.StudentID)
}
