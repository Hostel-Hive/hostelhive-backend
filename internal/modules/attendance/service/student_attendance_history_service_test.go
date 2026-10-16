package service_test

import (
	"context"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	"testing"
	"time"
)

type studentHistoryRepo struct {
	calls  int
	actor  string
	filter dto.StudentHistoryFilter
	err    error
}

func (f *studentHistoryRepo) OwnEvents(_ context.Context, actor string, q dto.StudentHistoryFilter) (d.StudentHistoryPage, error) {
	f.calls++
	f.actor = actor
	f.filter = q
	return d.StudentHistoryPage{}, f.err
}
func TestStudentHistoryValidation(t *testing.T) {
	loc := time.FixedZone("Asia/Colombo", 19800)
	for _, q := range []dto.StudentHistoryQuery{
		{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}, {Limit: 1, Offset: 1000001}, {Limit: 1, Direction: "in"}, {Limit: 1, Direction: "UNKNOWN"},
		{Limit: 1, From: "2026-10-01"}, {Limit: 1, To: "2026-10-01"},
		{Limit: 1, From: "2026-02-29", To: "2026-03-01"}, {Limit: 1, From: "2026-1-01", To: "2026-01-02"},
		{Limit: 1, From: "2026-10-02", To: "2026-10-01"}, {Limit: 1, From: "2026-01-01", To: "2027-01-02"},
		{Limit: 1, From: "0000-01-01", To: "0000-01-01"}, {Limit: 1, From: "9999-01-01", To: "9999-01-01"},
	} {
		repo := &studentHistoryRepo{}
		if _, e := svc.NewStudentHistory(repo, loc).Events(context.Background(), "student", q); !errors.Is(e, d.ErrInvalid) || repo.calls != 0 {
			t.Fatalf("invalid query accepted %+v: %v", q, e)
		}
	}
	repo := &studentHistoryRepo{}
	if _, e := svc.NewStudentHistory(repo, nil).Events(context.Background(), "student", dto.StudentHistoryQuery{Limit: 20}); !errors.Is(e, d.ErrInvalid) {
		t.Fatal("nil timezone")
	}
	if _, e := svc.NewStudentHistory(repo, loc).Events(context.Background(), "", dto.StudentHistoryQuery{Limit: 20}); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("empty identity")
	}
	repo.err = d.ErrUnavailable
	if _, e := svc.NewStudentHistory(repo, loc).Events(context.Background(), "verified", dto.StudentHistoryQuery{Limit: 20}); !errors.Is(e, d.ErrUnavailable) || repo.actor != "verified" || repo.filter.Start != nil || repo.filter.End != nil || repo.filter.Timezone != "Asia/Colombo" {
		t.Fatal("unbounded filter or error forwarding")
	}
	repo.err = nil
	if _, e := svc.NewStudentHistory(repo, loc).Events(context.Background(), "verified", dto.StudentHistoryQuery{Limit: 100, Offset: 1000000, From: "2024-01-01", To: "2024-12-31", Direction: "OUT"}); e != nil {
		t.Fatal("leap-year range", e)
	}
	if repo.filter.Start.UTC().Format(time.RFC3339) != "2023-12-31T18:30:00Z" || repo.filter.End.UTC().Format(time.RFC3339) != "2024-12-31T18:30:00Z" {
		t.Fatal("local boundaries")
	}
	// Calendar days, not 24-hour increments: the spring transition has 23 hours.
	ny, e := time.LoadLocation("America/New_York")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.NewStudentHistory(repo, ny).Events(context.Background(), "verified", dto.StudentHistoryQuery{Limit: 20, From: "2026-03-08", To: "2026-03-08"}); e != nil || repo.filter.End.Sub(*repo.filter.Start) != 23*time.Hour {
		t.Fatal("DST boundary", e)
	}
}
