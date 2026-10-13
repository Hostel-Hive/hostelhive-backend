package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
)

type RealtimePublisher interface {
	Publish(context.Context, domain.Projection) error
	SetGrant(context.Context, string, domain.Grant) error
}

type RealtimeRepository interface {
	GrantSession(context.Context, string, RealtimePublisher) (domain.RealtimeSession, error)
	PublicationStatus(context.Context, string) (domain.PublicationStatus, error)
	PublishOne(context.Context, RealtimePublisher) (bool, error)
	SyncGrant(context.Context, RealtimePublisher, string) (bool, error)
	Reconcile(context.Context) error
}

type Realtime struct {
	store       RealtimeRepository
	publisher   RealtimePublisher
	databaseURL string
}

func NewRealtime(store RealtimeRepository, publisher RealtimePublisher, databaseURL string) *Realtime {
	return &Realtime{store, publisher, databaseURL}
}

func (s *Realtime) Session(ctx context.Context, actor string) (domain.RealtimeSession, error) {
	if s.publisher == nil {
		return domain.RealtimeSession{}, domain.ErrUnavailable
	}
	session, err := s.store.GrantSession(ctx, actor, s.publisher)
	session.DatabaseURL = s.databaseURL
	session.Path = "hostelhive/attendance"
	session.RenewAfterSeconds = 120
	return session, err
}

func (s *Realtime) Status(ctx context.Context, actor string) (domain.PublicationStatus, error) {
	status, err := s.store.PublicationStatus(ctx, actor)
	status.Enabled = s.publisher != nil
	return status, err
}
