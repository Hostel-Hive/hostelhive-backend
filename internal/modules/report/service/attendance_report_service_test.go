package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
)

type capture struct {
	filter dto.AttendanceFilter
	actor  string
	err    error
}

func (c *capture) Movements(_ context.Context, a string, f dto.AttendanceFilter) (domain.MovementPage, error) {
	c.actor, c.filter = a, f
	return domain.MovementPage{}, c.err
}
func (c *capture) Daily(_ context.Context, a string, f dto.AttendanceFilter) (domain.DailyPage, error) {
	c.actor, c.filter = a, f
	return domain.DailyPage{}, c.err
}
func TestCalendarBoundariesAndForwarding(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	store := &capture{}
	s := NewAttendance(store, loc)
	q := dto.AttendanceQuery{From: "2026-10-14", To: "2026-10-15", Limit: 20}
	_, err = s.Movements(context.Background(), "admin", q)
	if err != nil || store.actor != "admin" || store.filter.Days != 2 ||
		store.filter.Start.UTC().Format(time.RFC3339) != "2026-10-13T18:30:00Z" ||
		store.filter.End.UTC().Format(time.RFC3339) != "2026-10-15T18:30:00Z" {
		t.Fatal(store, err)
	}
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	s = NewAttendance(store, ny)
	for _, tc := range []struct {
		date  string
		hours float64
	}{{"2026-03-08", 23}, {"2026-11-01", 25}} {
		q.From, q.To = tc.date, tc.date
		_, err = s.Daily(context.Background(), "warden", q)
		if err != nil || store.filter.End.Sub(store.filter.Start).Hours() != tc.hours || store.filter.Days != 1 {
			t.Fatal(tc, store.filter, err)
		}
	}
	q.From, q.To = "2024-01-01", "2024-12-31"
	if _, err = s.Daily(context.Background(), "warden", q); err != nil || store.filter.Days != 366 {
		t.Fatal("leap year", err)
	}
	store.err = domain.ErrUnavailable
	if _, err = s.Movements(context.Background(), "admin", q); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err = s.Daily(context.Background(), "admin", q); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestInvalidReportQueries(t *testing.T) {
	valid := dto.AttendanceQuery{From: "2026-10-14", To: "2026-10-15", Limit: 20}
	cases := []dto.AttendanceQuery{}
	for _, field := range []string{"from", "to", "direction", "student", "limit", "offset"} {
		q := valid
		switch field {
		case "from":
			q.From = "2026-02-30"
		case "to":
			q.To = ""
		case "direction":
			q.Direction = "in"
		case "student":
			q.StudentID = "00000000-0000-0000-0000-000000000000"
		case "limit":
			q.Limit = 101
		case "offset":
			q.Offset = -1
		}
		cases = append(cases, q)
	}
	for _, pair := range [][2]string{{"2026-10-15", "2026-10-14"}, {"2026-01-01", "2027-01-02"}, {"0000-01-01", "0000-01-01"}, {"2026-1-01", "2026-01-02"}, {"9999-12-31", "9999-12-31"}, {"1000-01-01", "9000-01-01"}} {
		q := valid
		q.From, q.To = pair[0], pair[1]
		cases = append(cases, q)
	}
	for _, id := range []string{"invalid", "{41757a7c-0897-48e0-ba1c-26a74ce447cd}", "41757A7C-0897-48E0-BA1C-26A74CE447CD"} {
		q := valid
		q.StudentID = id
		cases = append(cases, q)
	}
	for _, q := range cases {
		s := NewAttendance(&capture{}, time.UTC)
		if _, err := s.Movements(context.Background(), "admin", q); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(q, err)
		}
		if _, err := s.Daily(context.Background(), "admin", q); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(q, err)
		}
	}
	s := NewAttendance(&capture{}, nil)
	if _, err := s.Movements(context.Background(), "admin", valid); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
}
