package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
)

type DeviceRepository interface {
	Register(context.Context, string, string, string, []byte) (domain.Device, error)
	List(context.Context, string, int, int) (domain.DevicePage, error)
	Rotate(context.Context, string, string, []byte) (domain.Device, error)
	Deactivate(context.Context, string, string) (domain.Device, error)
	Authentication(context.Context, string) (domain.AuthenticationMaterial, error)
}
