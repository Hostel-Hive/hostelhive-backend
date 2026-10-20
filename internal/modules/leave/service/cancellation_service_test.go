package service

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
	"testing"
)

func TestCancellationValidation(t *testing.T) {
	f := &fake{}
	s := New(f)
	for _, id := range []string{"", "bad", "10000000-0000-4000-8000-00000000000g"} {
		if _, err := s.Cancel(context.Background(), "student", id); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(id, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid cancellation reached persistence")
	}
	if _, err := s.Cancel(context.Background(), "student", "AB000000-0000-4000-8000-000000000001"); err != nil || f.calls != 1 {
		t.Fatal(err, f.calls)
	}
	if _, err := s.List(context.Background(), "student", domain.Filter{Limit: 20, Status: "cancelled"}); err != nil {
		t.Fatal(err)
	}
}
