package service

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"strings"
	"testing"
	"time"
)

type exportSource struct {
	movement             domain.MovementPage
	daily                domain.DailyPage
	leave                domain.LeavePage
	complaint            domain.ComplaintPage
	err                  error
	actor                string
	limit, offset, calls int
}

func (s *exportSource) record(actor string, limit, offset int) {
	s.actor, s.limit, s.offset = actor, limit, offset
	s.calls++
}
func (s *exportSource) Movements(_ context.Context, a string, f dto.AttendanceFilter) (domain.MovementPage, error) {
	s.record(a, f.Limit, f.Offset)
	return s.movement, s.err
}
func (s *exportSource) Daily(_ context.Context, a string, f dto.AttendanceFilter) (domain.DailyPage, error) {
	s.record(a, f.Limit, f.Offset)
	return s.daily, s.err
}
func (s *exportSource) Requests(_ context.Context, a string, f dto.LeaveFilter) (domain.LeavePage, error) {
	s.record(a, f.Limit, f.Offset)
	return s.leave, s.err
}
func (s *exportSource) Records(_ context.Context, a string, f dto.ComplaintFilter) (domain.ComplaintPage, error) {
	s.record(a, f.Limit, f.Offset)
	return s.complaint, s.err
}

type exportWriter struct {
	doc   domain.ExportDocument
	err   error
	calls int
	empty bool
}

func (w *exportWriter) Build(_ context.Context, d domain.ExportDocument) ([]byte, error) {
	w.doc = d
	w.calls++
	if w.empty {
		return nil, w.err
	}
	return []byte("workbook"), w.err
}
func TestAllExportMappingsAndSnapshotFetch(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	by := "admin"
	access := "approved"
	source := &exportSource{
		movement:  domain.MovementPage{Metadata: domain.Metadata{Total: 1, Summary: domain.Totals{Entries: 1, DistinctStudents: 1}}, Items: []domain.Movement{{EventID: "event", StudentID: "student", Direction: "IN", OccurredAt: at, AccessResult: &access}}},
		daily:     domain.DailyPage{Metadata: domain.Metadata{Total: 2, Summary: domain.Totals{Entries: 1, DistinctStudents: 1}}, Items: []domain.Day{{Date: "2026-10-15", Totals: domain.Totals{Entries: 1, DistinctStudents: 1}}, {Date: "2026-10-14"}}},
		leave:     domain.LeavePage{Total: 2, Summary: domain.LeaveTotals{Pending: 1, Approved: 1}, Items: []domain.LeaveRequest{{LeaveID: "pending", StartsAt: at, EndsAt: at, CreatedAt: at, Status: "pending", Reason: "=SUM(1,2)"}, {LeaveID: "approved", StartsAt: at, EndsAt: at, CreatedAt: at, DecidedAt: &at, DecidedBy: &by, Status: "approved", DecisionNote: "+1+1"}}},
		complaint: domain.ComplaintPage{Total: 2, Summary: domain.ComplaintTotals{ByStatus: domain.ComplaintStatuses{InProgress: 1, Resolved: 1}, ByCategory: domain.ComplaintCategories{Maintenance: 2}}, Items: []domain.ComplaintRecord{{ComplaintID: "reopened", CreatedAt: at, UpdatedAt: at, Version: 9007199254740991, Status: "in_progress", Description: "@SUM(1,2)"}, {ComplaintID: "resolved", CreatedAt: at, UpdatedAt: at, Version: 2, Status: "resolved", Resolution: &domain.ComplaintResolution{UpdateID: "resolution", ResolvedBy: "staff", ResolvedAt: at, Note: "-1+1"}}}},
	}
	writer := &exportWriter{}
	s := NewExports(source, source, source, writer, loc)
	for _, tc := range []struct {
		kind domain.ReportKind
		rows int
	}{{domain.MovementReport, 1}, {domain.DailyReport, 2}, {domain.LeaveReport, 2}, {domain.ComplaintReport, 2}} {
		source.calls = 0
		file, err := s.Export(context.Background(), "verified-admin", tc.kind, dto.ExportQuery{From: "2026-10-14", To: "2026-10-15"})
		if err != nil || file.Name != "hostelhive-"+string(tc.kind)+"-2026-10-14-to-2026-10-15.xlsx" || string(file.Data) != "workbook" || source.calls != 1 || source.actor != "verified-admin" || source.limit != 5001 || source.offset != 0 || len(writer.doc.Rows) != tc.rows {
			t.Fatal(tc, file, source, writer.doc, err)
		}
		if writer.doc.Summary[0].Value != int64(tc.rows) || writer.doc.Summary[1].Value != int64(tc.rows) {
			t.Fatal(writer.doc.Summary)
		}
		switch tc.kind {
		case domain.MovementReport:
			if writer.doc.Rows[0][4].Text != "2026-10-15T05:30:00+05:30" || writer.doc.Rows[0][5].Text != "approved" || writer.doc.Rows[0][6].Text != "" {
				t.Fatal(writer.doc.Rows)
			}
		case domain.DailyReport:
			if *writer.doc.Rows[1][1].Number != 0 || writer.doc.Rows[1][0].Text != "2026-10-14" {
				t.Fatal(writer.doc.Rows)
			}
		case domain.LeaveReport:
			if writer.doc.Rows[0][6].Text != "=SUM(1,2)" || writer.doc.Rows[0][9].Text != "" || writer.doc.Rows[0][10].Text != "" || writer.doc.Rows[1][10].Text != "admin" {
				t.Fatal(writer.doc.Rows)
			}
		case domain.ComplaintReport:
			if writer.doc.Rows[0][7].Text != "9007199254740991" || writer.doc.Rows[0][10].Text != "No" || writer.doc.Rows[0][11].Text != "" || writer.doc.Rows[1][11].Text != "resolution" || writer.doc.Rows[1][14].Text != "-1+1" {
				t.Fatal(writer.doc.Rows)
			}
		}
	}
}
func TestExportLimitAndFailures(t *testing.T) {
	source := &exportSource{}
	writer := &exportWriter{}
	s := NewExports(source, source, source, writer, time.UTC)
	q := dto.ExportQuery{From: "2026-10-14", To: "2026-10-15"}
	for _, kind := range []domain.ReportKind{domain.MovementReport, domain.DailyReport, domain.LeaveReport, domain.ComplaintReport} {
		source.movement.Total = 5001
		source.daily.Total = 5001
		source.leave.Total = 5001
		source.complaint.Total = 5001
		if _, err := s.Export(context.Background(), "admin", kind, q); !errors.Is(err, domain.ErrExportTooLarge) || writer.calls != 0 {
			t.Fatal(kind, err, writer.calls)
		}
		source.movement.Total = 1
		source.daily.Total = 1
		source.leave.Total = 1
		source.complaint.Total = 1
		if _, err := s.Export(context.Background(), "admin", kind, q); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatal(kind, err)
		}
		for _, failure := range []error{domain.ErrForbidden, domain.ErrUnavailable} {
			source.err = failure
			if _, err := s.Export(context.Background(), "admin", kind, q); !errors.Is(err, failure) {
				t.Fatal(kind, err)
			}
		}
		source.err = nil
	}
	source.leave = domain.LeavePage{Total: 5000, Items: make([]domain.LeaveRequest, 5000)}
	if _, err := s.Export(context.Background(), "admin", domain.LeaveReport, q); err != nil || len(writer.doc.Rows) != 5000 {
		t.Fatal(err)
	}
	source.leave = domain.LeavePage{}
	for _, failure := range []error{errors.New("encoder failed"), nil} {
		writer.err = failure
		writer.empty = failure == nil
		if _, err := s.Export(context.Background(), "admin", domain.LeaveReport, q); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatal(err)
		}
	}
	writer.err = nil
	writer.empty = false
	s.writer = nil
	if _, err := s.Export(context.Background(), "admin", domain.LeaveReport, q); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Export(ctx, "admin", domain.LeaveReport, q); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := completeExport(-1, 0, false, domain.ExportRowLimit); err == nil {
		t.Fatal("negative total accepted")
	}
}
func TestExportValidation(t *testing.T) {
	source := &exportSource{}
	writer := &exportWriter{}
	s := NewExports(source, source, source, writer, time.UTC)
	valid := dto.ExportQuery{From: "2026-10-14", To: "2026-10-15"}
	for _, tc := range []struct {
		kind domain.ReportKind
		q    dto.ExportQuery
	}{
		{domain.MovementReport, dto.ExportQuery{From: valid.From, To: valid.To, Category: "maintenance"}},
		{domain.DailyReport, dto.ExportQuery{From: valid.From, To: valid.To, Status: "pending"}},
		{domain.MovementReport, dto.ExportQuery{From: valid.From, To: valid.To, Direction: "bad"}},
		{domain.LeaveReport, dto.ExportQuery{From: valid.From, To: valid.To, Category: "maintenance"}},
		{domain.LeaveReport, dto.ExportQuery{From: valid.From, To: valid.To, Status: "resolved"}},
		{domain.ComplaintReport, dto.ExportQuery{From: valid.From, To: valid.To, Direction: "IN"}},
		{domain.ComplaintReport, dto.ExportQuery{From: valid.From, To: valid.To, Category: "bad"}},
		{domain.ComplaintReport, dto.ExportQuery{}},
		{domain.ReportKind("unknown"), valid},
	} {
		if _, err := s.Export(context.Background(), "admin", tc.kind, tc.q); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(tc, err)
		}
	}
	for _, kind := range []domain.ReportKind{domain.MovementReport, domain.DailyReport, domain.LeaveReport, domain.ComplaintReport} {
		q := valid
		q.StudentID = strings.Repeat("x", 36)
		if _, err := s.Export(context.Background(), "admin", kind, q); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(kind, err)
		}
	}
	if source.calls != 0 || writer.calls != 0 {
		t.Fatal("invalid query reached adapters")
	}
}
