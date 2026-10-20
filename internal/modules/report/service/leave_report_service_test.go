package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
)

type leaveCapture struct {
	actor  string
	filter dto.LeaveFilter
	err    error
}

func (c *leaveCapture) Requests(_ context.Context, actor string, filter dto.LeaveFilter) (domain.LeavePage, error) {
	c.actor, c.filter = actor, filter
	return domain.LeavePage{Total: 7}, c.err
}
func TestLeaveReportFiltersAndDelegation(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	store := &leaveCapture{}
	s := NewLeaves(store, loc)
	q := dto.LeaveQuery{From: "2026-10-14", To: "2026-10-15", StudentID: "41757a7c-0897-48e0-ba1c-26a74ce447cd", Limit: 1, Offset: 2}
	for _, status := range []string{"", "pending", "approved", "rejected", "cancelled"} {
		q.Status = status
		page, err := s.Requests(context.Background(), "warden", q)
		if err != nil || page.Total != 7 || store.actor != "warden" || store.filter.LeaveQuery != q || store.filter.Timezone != "Asia/Colombo" || store.filter.Start.UTC().Format(time.RFC3339) != "2026-10-13T18:30:00Z" || store.filter.End.UTC().Format(time.RFC3339) != "2026-10-15T18:30:00Z" {
			t.Fatal(page, store, err)
		}
	}
	for _, failure := range []error{domain.ErrForbidden, domain.ErrUnavailable} {
		store.err = failure
		if _, err := s.Requests(context.Background(), "admin", q); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
}
func TestInvalidLeaveReportFilters(t *testing.T) {
	valid := dto.LeaveQuery{From: "2026-10-14", To: "2026-10-15", Limit: 20}
	for _, field := range []string{"status", "from", "to", "student", "limit", "offset"} {
		q := valid
		switch field {
		case "status":
			q.Status = "Approved"
		case "from":
			q.From = "2026-02-30"
		case "to":
			q.To = "2026-10-13"
		case "student":
			q.StudentID = "invalid"
		case "limit":
			q.Limit = 0
		case "offset":
			q.Offset = 1000001
		}
		if _, err := NewLeaves(&leaveCapture{}, time.UTC).Requests(context.Background(), "admin", q); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(field, err)
		}
	}
	if _, err := NewLeaves(&leaveCapture{}, nil).Requests(context.Background(), "admin", valid); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
}
