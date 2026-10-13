package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
)

type ImageObjects interface {
	Put(context.Context, string, string, []byte) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}
type ImageRead interface {
	Metadata() domain.Image
	Close()
}
type ImageWork interface {
	ImageRead
	Attach(context.Context) (domain.ImageResult, error)
}
type ImageRepository interface {
	CheckImageUpload(context.Context, string, string) error
	ReserveImage(context.Context, string, string, domain.Image) error
	BeginImage(context.Context, string, string, string, []byte) (ImageWork, error)
	BeginImageRead(context.Context, string, string, string) (ImageRead, error)
	ListImages(context.Context, string, string) (domain.ImageList, error)
	RemoveImage(context.Context, string, string, string) (domain.ImageRemoval, error)
	CleanupImageOne(context.Context, ImageObjects) (bool, error)
}
