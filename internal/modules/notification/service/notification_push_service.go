package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/notification/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"strings"
)

type PushDevices interface {
	RegisterPushDevice(context.Context, string, string, dto.PushRegistration) (domain.PushDevice, error)
	ListPushDevices(context.Context, string) (domain.PushDevicePage, error)
	RemovePushDevice(context.Context, string, string) error
}
type PushSend func(context.Context, domain.PushMessage) domain.PushResult

type PushQueue interface {
	DispatchPush(context.Context, PushSend) (int, error)
}
type PushSender interface {
	Send(context.Context, domain.PushMessage) domain.PushResult
}

type Push struct{ store PushDevices }

func NewPush(store PushDevices) *Push { return &Push{store} }
func (s *Push) RegisterPushDevice(ctx context.Context, uid, id string, v dto.PushRegistration) (domain.PushDevice, error) {
	if (!validation.UUID(id) || id == "00000000-0000-0000-0000-000000000000") || (v.Platform != "android" && v.Platform != "ios") || len(v.Token) < 20 || len(v.Token) > 4096 {
		return domain.PushDevice{}, domain.ErrInvalid
	}
	for _, r := range v.Token {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(":_-", r)) {
			return domain.PushDevice{}, domain.ErrInvalid
		}
	}
	return s.store.RegisterPushDevice(ctx, uid, strings.ToLower(id), v)
}
func (s *Push) ListPushDevices(ctx context.Context, uid string) (domain.PushDevicePage, error) {
	return s.store.ListPushDevices(ctx, uid)
}
func (s *Push) RemovePushDevice(ctx context.Context, uid, id string) error {
	if !validation.UUID(id) || id == "00000000-0000-0000-0000-000000000000" {
		return domain.ErrInvalid
	}
	return s.store.RemovePushDevice(ctx, uid, strings.ToLower(id))
}

type PushDispatcher struct {
	queue  PushQueue
	sender PushSender
}

func NewPushDispatcher(q PushQueue, s PushSender) *PushDispatcher { return &PushDispatcher{q, s} }
func (s *PushDispatcher) Dispatch(ctx context.Context) (int, error) {
	return s.queue.DispatchPush(ctx, s.sender.Send)
}
