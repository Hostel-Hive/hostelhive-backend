package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/leave/domain"
)

type allowanceStore struct {
	calls  int
	actor  string
	period domain.AllowancePeriod
	usage  domain.AllowanceUsage
	err    error
}

func (f *allowanceStore) AllowanceUsage(_ context.Context, actor string, period domain.AllowancePeriod) (domain.AllowanceUsage, error) {
	f.calls++
	f.actor, f.period = actor, period
	return f.usage, f.err
}

func TestAllowanceCalendarAndRemaining(t *testing.T) {
	f := &allowanceStore{usage: domain.AllowanceUsage{StudentID: "own-student"}}
	s := NewAllowance(f)
	for _, used := range []int{0, 7, 18, 20, 366} {
		f.usage.UsedDays = used
		v, err := s.GetAllowance(context.Background(), "verified-uid", 2026)
		if err != nil || v.StudentID != "own-student" || v.Year != 2026 || v.Timezone != "Asia/Colombo" || v.TotalDays != 18 || v.UsedDays != used || v.RemainingDays != max(18-used, 0) || v.ExceededDays != max(used-18, 0) {
			t.Fatal(v, err)
		}
		if f.actor != "verified-uid" || f.period.Start.UTC().Format(time.RFC3339) != "2025-12-31T18:30:00Z" || f.period.End.UTC().Format(time.RFC3339) != "2026-12-31T18:30:00Z" || f.period.Year != 2026 || f.period.Timezone != "Asia/Colombo" {
			t.Fatal(f.actor, f.period)
		}
	}
	for _, year := range []int{1, 2024, 9998} {
		if _, err := s.GetAllowance(context.Background(), "student", year); err != nil || f.period.Start.In(s.location).Year() != year || f.period.End.In(s.location).Year() != year+1 {
			t.Fatal(year, f.period, err)
		}
	}
}

func TestAllowanceDefaultsAtColomboNewYear(t *testing.T) {
	f := &allowanceStore{}
	s := NewAllowance(f)
	for _, tc := range []struct {
		instant string
		year    int
	}{
		{"2026-12-31T18:29:59Z", 2026},
		{"2026-12-31T18:30:00Z", 2027},
	} {
		now, err := time.Parse(time.RFC3339, tc.instant)
		if err != nil {
			t.Fatal(err)
		}
		s.now = func() time.Time { return now }
		v, err := s.GetAllowance(context.Background(), "student", 0)
		if err != nil || v.Year != tc.year || f.period.Year != tc.year {
			t.Fatal(v, err)
		}
	}
}

func TestAllowanceValidationAndErrors(t *testing.T) {
	f := &allowanceStore{}
	s := NewAllowance(f)
	for _, year := range []int{-1, 9999, 10000} {
		if _, err := s.GetAllowance(context.Background(), "student", year); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(year, err)
		}
	}
	if _, err := s.GetAllowance(context.Background(), " ", 2026); !errors.Is(err, domain.ErrForbidden) || f.calls != 0 {
		t.Fatal(err, f.calls)
	}
	for _, failure := range []error{domain.ErrForbidden, domain.ErrIneligible, domain.ErrUnavailable} {
		f.err = failure
		if _, err := s.GetAllowance(context.Background(), "student", 2026); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	s.location = nil
	if _, err := s.GetAllowance(context.Background(), "student", 2026); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
}
