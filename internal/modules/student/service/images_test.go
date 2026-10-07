package service

import (
	"bytes"
	"context"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func testPNG(width int) []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, width, 1)))
	return b.Bytes()
}
func TestImageNormalization(t *testing.T) {
	good := testPNG(2)
	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil)
	for _, tc := range []struct {
		name, mime string
		raw        []byte
		valid      bool
	}{{"png", "image/png", good, true}, {"jpeg", "image/jpeg", jpg.Bytes(), true}, {"metadata stripped", "image/png", append(append([]byte{}, good...), []byte("private trailing metadata")...), true}, {"mismatched type", "image/jpeg", good, false}, {"svg", "image/png", []byte("<svg/>"), false}, {"empty", "image/png", nil, false}, {"truncated", "image/png", good[:40], false}, {"oversized", "image/png", make([]byte, d.MaxImageBytes+1), false}, {"too wide", "image/png", testPNG(4097), false}} {
		t.Run(tc.name, func(t *testing.T) {
			data, m, err := normalizeImage(tc.raw, tc.mime)
			if (err == nil) != tc.valid {
				t.Fatal(err)
			}
			if tc.valid && (m.Size != len(data) || bytes.Contains(data, []byte("private trailing metadata"))) {
				t.Fatal("unsafe canonical image")
			}
		})
	}
}

type imageFixture struct {
	reserveErr, beginErr, putErr, attachErr error
	reserved, put, attached, closed         bool
	meta                                    d.Image
}

func (f *imageFixture) ReserveImage(_ context.Context, _, _ string, m d.Image) error {
	f.reserved = true
	f.meta = m
	return f.reserveErr
}
func (f *imageFixture) BeginImage(context.Context, string, string, string) (ImageWork, error) {
	return f, f.beginErr
}
func (f *imageFixture) Attach(context.Context) (d.Profile, error) {
	if !f.put {
		panic("attached before storage")
	}
	f.attached = true
	return d.Profile{}, f.attachErr
}
func (f *imageFixture) Close()                                            { f.closed = true }
func (f *imageFixture) GetImage(context.Context, string) (d.Image, error) { return f.meta, nil }
func (f *imageFixture) RemoveImage(context.Context, string, string) error { return nil }
func (f *imageFixture) CleanupOne(context.Context, Objects) (bool, error) { return false, nil }
func (f *imageFixture) Put(_ context.Context, key, mime string, b []byte) error {
	if !f.reserved {
		panic("upload before durable reservation")
	}
	f.put = true
	return f.putErr
}
func (f *imageFixture) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (f *imageFixture) Delete(context.Context, string) error        { return nil }
func TestImageUploadFailureOrdering(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		f                     imageFixture
		want                  error
		put, attached, closed bool
	}{{"success", imageFixture{}, nil, true, true, true}, {"reserve", imageFixture{reserveErr: d.ErrForbidden}, d.ErrForbidden, false, false, false}, {"begin", imageFixture{beginErr: d.ErrNotFound}, d.ErrNotFound, false, false, false}, {"storage", imageFixture{putErr: errors.New("secret")}, d.ErrUnavailable, true, false, true}, {"commit", imageFixture{attachErr: d.ErrUnavailable}, d.ErrUnavailable, true, true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			_, err := NewImages(&f, &f).Upload(context.Background(), "actor", "11111111-1111-1111-1111-111111111111", "image/png", testPNG(2))
			if !errors.Is(err, tc.want) || f.put != tc.put || f.attached != tc.attached || f.closed != tc.closed {
				t.Fatalf("ordering: %+v %v", f, err)
			}
			if !strings.HasPrefix(f.meta.Key, "student-images/") {
				t.Fatal("unsafe key")
			}
		})
	}
	f := &imageFixture{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewImages(f, f).Upload(ctx, "a", "id", "image/png", testPNG(2)); !errors.Is(err, d.ErrUnavailable) || f.reserved {
		t.Fatal("cancelled upload reserved data")
	}
}
