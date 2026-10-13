package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/imagevalidation"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"strings"
)

type Images struct {
	repo    ImageRepository
	objects ImageObjects
}

func NewImages(r ImageRepository, o ImageObjects) *Images { return &Images{r, o} }
func imageIDs(complaint, id string) bool                  { return validation.UUID(complaint) && validation.UUID(id) }
func (s *Images) Upload(ctx context.Context, actor, complaint, id, mime string, raw []byte) (domain.ImageResult, error) {
	if !imageIDs(complaint, id) {
		return domain.ImageResult{}, domain.ErrInvalid
	}
	complaint = strings.ToLower(complaint)
	id = strings.ToLower(id)
	if s.objects == nil {
		if err := s.repo.CheckImageUpload(ctx, actor, complaint); err != nil {
			return domain.ImageResult{}, err
		}
		return domain.ImageResult{}, domain.ErrUnavailable
	}
	data, meta, err := imagevalidation.Normalize(raw, mime)
	if err != nil {
		return domain.ImageResult{}, domain.ErrInvalid
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return domain.ImageResult{}, domain.ErrUnavailable
	}
	hash := sha256.Sum256(append([]byte(mime), data...))
	m := domain.Image{ImageID: id, ComplaintID: complaint, ContentType: mime, Size: meta.Size, Width: meta.Width, Height: meta.Height, Hash: hash[:], Key: "complaint-images/" + complaint + "/" + id + "/" + hex.EncodeToString(nonce[:])}
	if err = s.repo.ReserveImage(ctx, actor, complaint, m); err != nil {
		return domain.ImageResult{}, err
	}
	work, err := s.repo.BeginImage(ctx, actor, complaint, id, hash[:])
	if err != nil {
		return domain.ImageResult{}, err
	}
	defer work.Close()
	stored := work.Metadata()
	if stored.State == "attached" {
		return domain.ImageResult{Image: stored, Version: *stored.AttachedVersion, Replayed: true}, nil
	}
	if s.objects.Put(ctx, stored.Key, mime, data) != nil {
		return domain.ImageResult{}, domain.ErrUnavailable
	}
	return work.Attach(ctx)
}
func (s *Images) Get(ctx context.Context, actor, complaint, id string) ([]byte, domain.Image, error) {
	if !imageIDs(complaint, id) {
		return nil, domain.Image{}, domain.ErrInvalid
	}
	work, err := s.repo.BeginImageRead(ctx, actor, strings.ToLower(complaint), strings.ToLower(id))
	if err != nil {
		return nil, domain.Image{}, err
	}
	defer work.Close()
	m := work.Metadata()
	if s.objects == nil {
		return nil, m, domain.ErrUnavailable
	}
	data, err := s.objects.Get(ctx, m.Key)
	if err != nil || len(data) != m.Size {
		return nil, m, domain.ErrUnavailable
	}
	hash := sha256.Sum256(append([]byte(m.ContentType), data...))
	if string(hash[:]) != string(m.Hash) {
		return nil, m, domain.ErrUnavailable
	}
	return data, m, nil
}
func (s *Images) List(ctx context.Context, actor, complaint string) (domain.ImageList, error) {
	if !validation.UUID(complaint) {
		return domain.ImageList{}, domain.ErrInvalid
	}
	return s.repo.ListImages(ctx, actor, strings.ToLower(complaint))
}
func (s *Images) Remove(ctx context.Context, actor, complaint, id string) (domain.ImageRemoval, error) {
	if !imageIDs(complaint, id) {
		return domain.ImageRemoval{}, domain.ErrInvalid
	}
	return s.repo.RemoveImage(ctx, actor, strings.ToLower(complaint), strings.ToLower(id))
}
