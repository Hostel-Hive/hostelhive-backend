package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/imagevalidation"
	"io"
	"strings"
)

type Images struct {
	repo    ImageRepository
	objects Objects
}

func NewImages(r ImageRepository, o Objects) *Images { return &Images{r, o} }
func normalizeImage(raw []byte, mime string) ([]byte, d.Image, error) {
	data, meta, err := imagevalidation.Normalize(raw, mime)
	if err != nil {
		return nil, d.Image{}, d.ErrInvalid
	}
	return data, d.Image{ContentType: meta.ContentType, Size: meta.Size, Width: meta.Width, Height: meta.Height}, nil
}
func (s *Images) Upload(ctx context.Context, actor, id, mime string, raw []byte) (d.Profile, error) {
	data, meta, err := normalizeImage(raw, mime)
	if err != nil {
		return d.Profile{}, err
	}
	if err = ctx.Err(); err != nil {
		return d.Profile{}, d.ErrUnavailable
	}
	var nonce [16]byte
	if _, err = io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return d.Profile{}, d.ErrUnavailable
	}
	meta.Key = "student-images/" + strings.ToLower(id) + "/" + hex.EncodeToString(nonce[:])
	if err = s.repo.ReserveImage(ctx, actor, id, meta); err != nil {
		return d.Profile{}, err
	}
	w, err := s.repo.BeginImage(ctx, actor, id, meta.Key)
	if err != nil {
		return d.Profile{}, err
	}
	defer w.Close()
	if s.objects.Put(ctx, meta.Key, meta.ContentType, data) != nil {
		return d.Profile{}, d.ErrUnavailable
	}
	return w.Attach(ctx)
}
func (s *Images) Get(ctx context.Context, id string) ([]byte, d.Image, error) {
	meta, err := s.repo.GetImage(ctx, id)
	if err != nil {
		return nil, meta, err
	}
	data, err := s.objects.Get(ctx, meta.Key)
	if err != nil || len(data) != meta.Size {
		return nil, meta, d.ErrUnavailable
	}
	return data, meta, nil
}
func (s *Images) Remove(ctx context.Context, actor, id string) error {
	return s.repo.RemoveImage(ctx, actor, id)
}
