package service

import (
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"strconv"
	"time"
)

type Exports struct {
	attendance *Attendance
	leaves     *Leaves
	complaints *Complaints
	writer     ExportDocumentWriter
	location   *time.Location
	rowLimit   int
	suffix     string
}

func NewExports(a AttendanceRepository, l LeaveRepository, c ComplaintRepository, w ExportDocumentWriter, loc *time.Location) *Exports {
	return &Exports{NewAttendance(a, loc), NewLeaves(l, loc), NewComplaints(c, loc), w, loc, domain.ExportRowLimit, ".xlsx"}
}
func textRow(values ...string) []domain.ExportCell {
	row := make([]domain.ExportCell, len(values))
	for i, v := range values {
		row[i].Text = v
	}
	return row
}
func numberCell(value int64) domain.ExportCell { return domain.ExportCell{Number: &value} }
func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (s *Exports) timestamp(v time.Time) string { return v.In(s.location).Format(time.RFC3339Nano) }
func (s *Exports) optionalTime(v *time.Time) string {
	if v == nil {
		return ""
	}
	return s.timestamp(*v)
}
func completeExport(total int64, rows int, more bool, limit int) error {
	if total > int64(limit) {
		return domain.ErrExportTooLarge
	}
	if total < 0 || int64(rows) != total || more {
		return domain.ErrUnavailable
	}
	return nil
}
func (s *Exports) Export(ctx context.Context, actor string, kind domain.ReportKind, q dto.ExportQuery) (domain.ExportFile, error) {
	var file domain.ExportFile
	if ctx.Err() != nil {
		return file, domain.ErrUnavailable
	}
	doc := domain.ExportDocument{Rows: [][]domain.ExportCell{}}
	var total int64
	switch kind {
	case domain.MovementReport, domain.DailyReport:
		if q.Category != "" || q.Status != "" {
			return file, domain.ErrInvalid
		}
		f, err := s.attendance.filter(dto.AttendanceQuery{From: q.From, To: q.To, StudentID: q.StudentID, Direction: q.Direction, Limit: 20})
		if err != nil {
			return file, err
		}
		f.Limit = s.rowLimit + 1
		if kind == domain.MovementReport {
			p, err := s.attendance.store.Movements(ctx, actor, f)
			if err != nil {
				return file, err
			}
			if err = completeExport(p.Total, len(p.Items), p.HasMore, s.rowLimit); err != nil {
				return file, err
			}
			total = p.Total
			doc.Title = "Attendance movements"
			doc.Headers = []string{"Event ID", "Device ID", "Student ID", "Direction", "Occurred At", "Access Result", "Late Access ID", "Incident ID"}
			for _, v := range p.Items {
				doc.Rows = append(doc.Rows, textRow(v.EventID, v.DeviceID, v.StudentID, v.Direction, s.timestamp(v.OccurredAt), stringValue(v.AccessResult), stringValue(v.LateAccessID), stringValue(v.IncidentID)))
			}
			doc.Summary = attendanceMetrics(p.Summary)
		} else {
			p, err := s.attendance.store.Daily(ctx, actor, f)
			if err != nil {
				return file, err
			}
			if err = completeExport(p.Total, len(p.Items), p.HasMore, s.rowLimit); err != nil {
				return file, err
			}
			total = p.Total
			doc.Title = "Daily attendance totals"
			doc.Headers = []string{"Date", "Entries", "Exits", "Distinct Students"}
			for _, v := range p.Items {
				doc.Rows = append(doc.Rows, []domain.ExportCell{{Text: v.Date}, numberCell(v.Entries), numberCell(v.Exits), numberCell(v.DistinctStudents)})
			}
			doc.Summary = attendanceMetrics(p.Summary)
		}
		doc.Metadata = append(doc.Metadata, [2]string{"Direction", allFilter(q.Direction)}, [2]string{"Date Basis", "Recorded movement time; daily rows include zero-activity dates"})
	case domain.LeaveReport:
		if q.Category != "" || q.Direction != "" {
			return file, domain.ErrInvalid
		}
		f, err := s.leaves.filter(dto.LeaveQuery{From: q.From, To: q.To, StudentID: q.StudentID, Status: q.Status, Limit: 20})
		if err != nil {
			return file, err
		}
		f.Limit = s.rowLimit + 1
		p, err := s.leaves.store.Requests(ctx, actor, f)
		if err != nil {
			return file, err
		}
		if err = completeExport(p.Total, len(p.Items), p.HasMore, s.rowLimit); err != nil {
			return file, err
		}
		total = p.Total
		doc.Title = "Student leave requests"
		doc.Headers = []string{"Leave ID", "Request ID", "Student ID", "Submitted By", "Starts At", "Ends At", "Reason", "Status", "Created At", "Decided At", "Decided By", "Decision Note"}
		for _, v := range p.Items {
			doc.Rows = append(doc.Rows, textRow(v.LeaveID, v.RequestID, v.StudentID, v.SubmittedBy, s.timestamp(v.StartsAt), s.timestamp(v.EndsAt), v.Reason, v.Status, s.timestamp(v.CreatedAt), s.optionalTime(v.DecidedAt), stringValue(v.DecidedBy), v.DecisionNote))
		}
		doc.Summary = []domain.ExportMetric{{Name: "Pending", Value: p.Summary.Pending}, {Name: "Approved", Value: p.Summary.Approved}, {Name: "Rejected", Value: p.Summary.Rejected}}
		doc.Metadata = append(doc.Metadata, [2]string{"Status", allFilter(q.Status)}, [2]string{"Date Basis", "Request submission time"}, [2]string{"Status Basis", "Current request status at query snapshot"})
	case domain.ComplaintReport:
		if q.Direction != "" {
			return file, domain.ErrInvalid
		}
		f, err := s.complaints.filter(dto.ComplaintQuery{From: q.From, To: q.To, StudentID: q.StudentID, Category: q.Category, Status: q.Status, Limit: 20})
		if err != nil {
			return file, err
		}
		f.Limit = s.rowLimit + 1
		p, err := s.complaints.store.Records(ctx, actor, f)
		if err != nil {
			return file, err
		}
		if err = completeExport(p.Total, len(p.Items), p.HasMore, s.rowLimit); err != nil {
			return file, err
		}
		total = p.Total
		doc.Title = "Student complaints"
		doc.Headers = []string{"Complaint ID", "Request ID", "Student ID", "Submitted By", "Category", "Description", "Status", "Version", "Created At", "Updated At", "Resolution Recorded", "Resolution Update ID", "Resolved At", "Resolved By", "Resolution Note"}
		for _, v := range p.Items {
			recorded, update, at, by, note := "No", "", "", "", ""
			if v.Resolution != nil {
				recorded, update, at, by, note = "Yes", v.Resolution.UpdateID, s.timestamp(v.Resolution.ResolvedAt), v.Resolution.ResolvedBy, v.Resolution.Note
			}
			doc.Rows = append(doc.Rows, textRow(v.ComplaintID, v.RequestID, v.StudentID, v.SubmittedBy, v.Category, v.Description, v.Status, strconv.FormatInt(v.Version, 10), s.timestamp(v.CreatedAt), s.timestamp(v.UpdatedAt), recorded, update, at, by, note))
		}
		doc.Summary = []domain.ExportMetric{{Name: "Pending", Value: p.Summary.ByStatus.Pending}, {Name: "In Progress", Value: p.Summary.ByStatus.InProgress}, {Name: "Resolved", Value: p.Summary.ByStatus.Resolved}, {Name: "Maintenance", Value: p.Summary.ByCategory.Maintenance}, {Name: "Disciplinary", Value: p.Summary.ByCategory.Disciplinary}}
		doc.Metadata = append(doc.Metadata, [2]string{"Category", allFilter(q.Category)}, [2]string{"Status", allFilter(q.Status)}, [2]string{"Date Basis", "Complaint submission time"}, [2]string{"Status Basis", "Current complaint status; category/status totals are independent breakdowns"})
	default:
		return file, domain.ErrInvalid
	}
	doc.Metadata = append([][2]string{{"Report", doc.Title}, {"From", q.From}, {"To", q.To}, {"Timezone", s.location.String()}, {"Student ID", allFilter(q.StudentID)}}, doc.Metadata...)
	doc.Metadata = append(doc.Metadata, [2]string{"Timestamps", "RFC3339 text in hostel timezone; original instants preserved"}, [2]string{"Blank Values", "Empty, not recorded or not applicable; no missing decision/permission/resolution is inferred"})
	doc.Summary = append([]domain.ExportMetric{{Name: "Total Matching Rows", Value: total}, {Name: "Exported Rows", Value: int64(len(doc.Rows))}, {Name: "Export Row Limit", Value: int64(s.rowLimit)}}, doc.Summary...)
	if s.writer == nil || ctx.Err() != nil {
		return file, domain.ErrUnavailable
	}
	data, err := s.writer.Build(ctx, doc)
	if errors.Is(err, domain.ErrExportTooLarge) {
		return file, domain.ErrExportTooLarge
	}
	if err != nil || ctx.Err() != nil || len(data) == 0 {
		return file, domain.ErrUnavailable
	}
	file.Name = "hostelhive-" + string(kind) + "-" + q.From + "-to-" + q.To + s.suffix
	file.Data = data
	return file, nil
}
func attendanceMetrics(v domain.Totals) []domain.ExportMetric {
	return []domain.ExportMetric{{Name: "Entries", Value: v.Entries}, {Name: "Exits", Value: v.Exits}, {Name: "Distinct Students", Value: v.DistinctStudents}}
}
func allFilter(v string) string {
	if v == "" {
		return "All"
	}
	return v
}

// NewPDFExports reuses the same snapshot queries and source-to-document mapping as Excel.
func NewPDFExports(a AttendanceRepository, l LeaveRepository, c ComplaintRepository, w ExportDocumentWriter, loc *time.Location) *Exports {
	s := NewExports(a, l, c, w, loc)
	s.rowLimit, s.suffix = domain.PDFExportRowLimit, ".pdf"
	return s
}
