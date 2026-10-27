package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
)

type passStore struct {
	calls  int
	actor  string
	filter domain.PassFilter
	err    error
}

func (s *passStore) OwnPasses(_ context.Context, actor string, f domain.PassFilter) (domain.PassPage, error) {
	s.calls++
	s.actor, s.filter = actor, f
	return domain.PassPage{Items: []domain.PassItem{}, Limit: f.Limit, Offset: f.Offset}, s.err
}

func TestPassHistoryValidation(t *testing.T) {
	store := &passStore{}
	s := NewPassHistory(store)
	ctx := context.Background()
	for _, f := range []domain.PassFilter{
		{Limit: 20}, {Type: "leave", Status: "cancelled", Limit: 1, Offset: 1000000},
		{Type: "late_access", Status: "pending", Limit: 100},
		{Status: "approved", Limit: 20}, {Status: "rejected", Limit: 20},
	} {
		v, err := s.ListPasses(ctx, "own-uid", f)
		if err != nil || store.filter != f || store.actor != "own-uid" || v.Limit != f.Limit || v.Offset != f.Offset {
			t.Fatal(f, v, err, store)
		}
	}
	before := store.calls
	for _, f := range []domain.PassFilter{
		{}, {Limit: -1}, {Limit: 101}, {Limit: 20, Offset: -1}, {Limit: 20, Offset: 1000001},
		{Limit: 20, Type: "LEAVE"}, {Limit: 20, Type: "going_home"},
		{Limit: 20, Status: "consumed"}, {Limit: 20, Status: "CANCELLED"},
	} {
		if _, err := s.ListPasses(ctx, "own-uid", f); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(f, err)
		}
	}
	for _, actor := range []string{"", " "} {
		if _, err := s.ListPasses(ctx, actor, domain.PassFilter{Limit: 20}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal(actor, err)
		}
	}
	if before != store.calls {
		t.Fatal("invalid input reached repository")
	}
	store.err = errors.New("database unavailable")
	if _, err := s.ListPasses(ctx, "own-uid", domain.PassFilter{Limit: 20}); err != store.err {
		t.Fatal("repository error was lost", err)
	}
}
