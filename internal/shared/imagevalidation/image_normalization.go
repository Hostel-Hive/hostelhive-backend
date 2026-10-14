package imagevalidation

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
)

const MaxBytes = 5 * 1024 * 1024

var ErrInvalid = errors.New("invalid image")

type Metadata struct {
	ContentType         string
	Size, Width, Height int
}
type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(v []byte) (int, error) {
	if b.Len()+len(v) > MaxBytes {
		return 0, ErrInvalid
	}
	return b.Buffer.Write(v)
}

// Decode and re-encode approved formats; filenames and embedded metadata are not stored.
func Normalize(raw []byte, contentType string) ([]byte, Metadata, error) {
	var meta Metadata
	if len(raw) == 0 || len(raw) > MaxBytes {
		return nil, meta, ErrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 12000000 {
		return nil, meta, ErrInvalid
	}
	if (format != "jpeg" && format != "png") || contentType != "image/"+map[string]string{"jpeg": "jpeg", "png": "png"}[format] {
		return nil, meta, ErrInvalid
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, meta, ErrInvalid
	}
	var out cappedBuffer
	if format == "jpeg" {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, img)
	}
	if err != nil {
		return nil, meta, ErrInvalid
	}
	meta = Metadata{ContentType: contentType, Size: out.Len(), Width: cfg.Width, Height: cfg.Height}
	return out.Bytes(), meta, nil
}
