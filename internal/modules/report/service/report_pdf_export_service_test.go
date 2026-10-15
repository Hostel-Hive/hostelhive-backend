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

func TestPDFExportReusesSnapshotMappingWithOwnBounds(t *testing.T) {
	source := &exportSource{}
	writer := &exportWriter{}
	s := NewPDFExports(source, source, source, writer, time.UTC)
	q := dto.ExportQuery{From: "2026-10-01", To: "2026-10-15"}
	for _, kind := range []domain.ReportKind{domain.MovementReport, domain.DailyReport, domain.LeaveReport, domain.ComplaintReport} {
		source.calls = 0
		file, err := s.Export(context.Background(), "admin", kind, q)
		if err != nil || !strings.HasSuffix(file.Name, ".pdf") || source.limit != 1001 || source.calls != 1 || writer.doc.Summary[2].Value != 1000 {
			t.Fatal(kind, file, source, writer.doc, err)
		}
	}
	source.complaint = domain.ComplaintPage{Total: 1000, Items: make([]domain.ComplaintRecord, 1000)}
	if _, err := s.Export(context.Background(), "admin", domain.ComplaintReport, q); err != nil || len(writer.doc.Rows) != 1000 {
		t.Fatal(err)
	}
	source.complaint.Total = 1001
	if _, err := s.Export(context.Background(), "admin", domain.ComplaintReport, q); !errors.Is(err, domain.ErrExportTooLarge) {
		t.Fatal(err)
	}
	source.complaint = domain.ComplaintPage{}
	writer.err = domain.ErrExportTooLarge
	if _, err := s.Export(context.Background(), "admin", domain.ComplaintReport, q); !errors.Is(err, domain.ErrExportTooLarge) {
		t.Fatal("renderer page/byte limit not preserved", err)
	}
}
