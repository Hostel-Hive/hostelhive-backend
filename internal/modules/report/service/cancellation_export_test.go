package service

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"testing"
	"time"
)

func TestCancelledLeaveExportAudit(t *testing.T) {
	at := time.Date(2026, 10, 20, 10, 30, 0, 0, time.UTC)
	by := "student"
	source := &exportSource{leave: domain.LeavePage{Total: 1, Summary: domain.LeaveTotals{Cancelled: 1}, Items: []domain.LeaveRequest{{Status: "cancelled", StartsAt: at, EndsAt: at, CreatedAt: at, CancelledAt: &at, CancelledBy: &by}}}}
	for _, pdf := range []bool{false, true} {
		w := &exportWriter{}
		s := NewExports(source, source, source, w, time.UTC)
		if pdf {
			s = NewPDFExports(source, source, source, w, time.UTC)
		}
		_, err := s.Export(context.Background(), "admin", domain.LeaveReport, dto.ExportQuery{From: "2026-10-20", To: "2026-10-20", Status: "cancelled"})
		if err != nil {
			t.Fatal(err)
		}
		if len(w.doc.Headers) != 14 || w.doc.Headers[12] != "Cancelled At" || w.doc.Headers[13] != "Cancelled By" {
			t.Fatal(w.doc.Headers)
		}
		row := w.doc.Rows[0]
		if row[7].Text != "cancelled" || row[9].Text != "" || row[10].Text != "" || row[12].Text != "2026-10-20T10:30:00Z" || row[13].Text != "student" {
			t.Fatal(row)
		}
		found := false
		for _, m := range w.doc.Summary {
			if m.Name == "Cancelled" && m.Value == 1 {
				found = true
			}
		}
		if !found {
			t.Fatal(w.doc.Summary)
		}
	}
}
