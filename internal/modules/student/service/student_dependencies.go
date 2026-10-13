package service

import (
	"context"

	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	sdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
)

type Repository interface {
	List(context.Context, sdomain.Filter) (sdomain.Page, error)
	Get(context.Context, string) (sdomain.Profile, error)
	Create(context.Context, string, sdto.CreateInput) (sdomain.Profile, error)
	Update(context.Context, string, string, sdto.Details) (sdomain.Profile, error)
	Delete(context.Context, string, string) error
}

// The repository must recheck active Admin authority inside each transaction.
type ImportRepository interface {
	CreateImport(context.Context, string, sdto.CreateInput) (sdomain.Profile, error)
}

type Objects interface {
	Put(context.Context, string, string, []byte) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}

// QRRepository checks current actor authority and target eligibility in the
// issuance transaction. Empty studentID means the actor's own student profile.
type QRRepository interface {
	QR(context.Context, string, string, string) (string, error)
}

type ImageWork interface {
	Attach(context.Context) (sdomain.Profile, error)
	Close()
}

type ImageRepository interface {
	ReserveImage(context.Context, string, string, sdomain.Image) error
	BeginImage(context.Context, string, string, string) (ImageWork, error)
	GetImage(context.Context, string) (sdomain.Image, error)
	RemoveImage(context.Context, string, string) error
	CleanupOne(context.Context, Objects) (bool, error)
}
