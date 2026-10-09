package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
)

type Objects interface {
	Put(context.Context, string, string, []byte) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}
type ImageWork interface {
	Attach(context.Context) (d.Profile, error)
	Close()
}
type ImageRepository interface {
	ReserveImage(context.Context, string, string, d.Image) error
	BeginImage(context.Context, string, string, string) (ImageWork, error)
	GetImage(context.Context, string) (d.Image, error)
	RemoveImage(context.Context, string, string) error
	CleanupOne(context.Context, Objects) (bool, error)
}
type Images struct {
	repo    ImageRepository
	objects Objects
}

func NewImages(r ImageRepository, o Objects) *Images { return &Images{r, o} }

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(v []byte) (int, error) {
	if b.Len()+len(v) > d.MaxImageBytes {
		return 0, d.ErrInvalid
	}
	return b.Buffer.Write(v)
}

// Decode and re-encode approved formats; filenames and embedded metadata are not stored.
func normalizeImage(raw []byte, contentType string) ([]byte, d.Image, error) {
	var meta d.Image
	if len(raw) == 0 || len(raw) > d.MaxImageBytes {
		return nil, meta, d.ErrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 12000000 {
		return nil, meta, d.ErrInvalid
	}
	if (format != "jpeg" && format != "png") || contentType != "image/"+map[string]string{"jpeg": "jpeg", "png": "png"}[format] {
		return nil, meta, d.ErrInvalid
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, meta, d.ErrInvalid
	}
	var out cappedBuffer
	if format == "jpeg" {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, img)
	}
	if err != nil {
		return nil, meta, d.ErrInvalid
	}
	meta = d.Image{ContentType: contentType, Size: out.Len(), Width: cfg.Width, Height: cfg.Height}
	return out.Bytes(), meta, nil
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
