package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/dto"
	svc "github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
)

type fakeDashboard struct {
	calls  int
	query  string
	filter dto.PresenceFilter
}

func (f *fakeDashboard) PresenceStudents(_ context.Context, _ string, filter dto.PresenceFilter) (d.PresencePage, error) {
	f.calls++
	f.filter = filter
	return d.PresencePage{}, d.ErrUnavailable
}
func (f *fakeDashboard) PresenceSummary(_ context.Context, _ string, query string) (d.PresenceSummary, error) {
	f.calls++
	f.query = query
	return d.PresenceSummary{}, d.ErrUnavailable
}

func TestDashboardValidatesQueryBeforePersistence(t *testing.T) {
	f := &fakeDashboard{}
	service := svc.NewDashboard(f)
	ctx := context.Background()
	for _, filter := range []dto.PresenceFilter{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}, {Limit: 1, Offset: 1000001}, {Limit: 1, Status: "in"}, {Limit: 1, Status: "SIDE"}, {Limit: 1, Query: strings.Repeat("a", 201)}, {Limit: 1, Query: "a\nb"}, {Limit: 1, Query: string([]byte{0xff})}} {
		if _, err := service.PresenceStudents(ctx, "admin", filter); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid filter accepted", filter, err)
		}
	}
	for _, query := range []string{strings.Repeat("界", 201), "x\x00y", string([]byte{0xff})} {
		if _, err := service.PresenceSummary(ctx, "admin", query); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid summary search")
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid input reached database")
	}
	_, err := service.PresenceStudents(ctx, "admin", dto.PresenceFilter{Query: "  Ravi  ", Status: "UNKNOWN", Limit: 100, Offset: 1000000})
	if !errors.Is(err, d.ErrUnavailable) || f.filter.Query != "Ravi" {
		t.Fatal("search not normalized or error lost")
	}
	_, err = service.PresenceSummary(ctx, "admin", "  Ravi  ")
	if !errors.Is(err, d.ErrUnavailable) || f.query != "Ravi" {
		t.Fatal("summary normalization/error")
	}
}
