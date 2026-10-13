package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	"time"
)

type DeviceRepository interface {
	Register(context.Context, string, string, string, []byte) (domain.Device, error)
	List(context.Context, string, int, int) (domain.DevicePage, error)
	Rotate(context.Context, string, string, []byte) (domain.Device, error)
	Deactivate(context.Context, string, string) (domain.Device, error)
	Authentication(context.Context, string) (domain.AuthenticationMaterial, error)
}

type ScanRepository interface {
	SetDirection(context.Context, string, string, string) (domain.Device, error)
	Record(context.Context, domain.Device, string, string, []byte, time.Duration) (domain.Event, error)
	Presence(context.Context, string, string) (domain.Presence, error)
	Events(context.Context, string, string, int, int) (domain.EventPage, error)
}

type DashboardRepository interface {
	PresenceStudents(context.Context, string, dto.PresenceFilter) (domain.PresencePage, error)
	PresenceSummary(context.Context, string, string) (domain.PresenceSummary, error)
}
