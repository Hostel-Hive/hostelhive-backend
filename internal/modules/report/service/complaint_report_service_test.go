package service

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"testing"
	"time"
)

type complaintCapture struct {
	actor  string
	filter dto.ComplaintFilter
	err    error
}

func (c *complaintCapture) Records(_ context.Context, actor string, f dto.ComplaintFilter) (domain.ComplaintPage, error) {
	c.actor, c.filter = actor, f
	return domain.ComplaintPage{Total: 7}, c.err
}
func TestComplaintReportFiltersAndForwarding(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	store := &complaintCapture{}
	s := NewComplaints(store, loc)
	q := dto.ComplaintQuery{From: "2026-10-14", To: "2026-10-15", StudentID: "41757a7c-0897-48e0-ba1c-26a74ce447cd", Limit: 1, Offset: 2}
	for _, category := range []string{"", "maintenance", "disciplinary"} {
		for _, status := range []string{"", "pending", "in_progress", "resolved"} {
			q.Category, q.Status = category, status
			page, err := s.Records(context.Background(), "warden", q)
			if err != nil || page.Total != 7 || store.actor != "warden" || store.filter.ComplaintQuery != q || store.filter.Timezone != "Asia/Colombo" || store.filter.Start.UTC().Format(time.RFC3339) != "2026-10-13T18:30:00Z" || store.filter.End.UTC().Format(time.RFC3339) != "2026-10-15T18:30:00Z" {
				t.Fatal(store, page, err)
			}
		}
	}
	for _, failure := range []error{domain.ErrForbidden, domain.ErrUnavailable} {
		store.err = failure
		if _, err := s.Records(context.Background(), "admin", q); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
}
func TestInvalidComplaintReportFilters(t *testing.T) {
	valid := dto.ComplaintQuery{From: "2026-10-14", To: "2026-10-15", Limit: 20}
	for _, field := range []string{"category", "status", "from", "to", "student", "limit", "offset"} {
		q := valid
		switch field {
		case "category":
			q.Category = "Maintenance"
		case "status":
			q.Status = "Resolved"
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
		if _, err := NewComplaints(&complaintCapture{}, time.UTC).Records(context.Background(), "admin", q); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(field, err)
		}
	}
	if _, err := NewComplaints(&complaintCapture{}, nil).Records(context.Background(), "admin", valid); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
}
