package domain

import "errors"

const ExportRowLimit = 5000
const PDFExportRowLimit = 1000
const PDFExportPageLimit = 200
const PDFExportByteLimit = 16 << 20
const PDFContentType = "application/pdf"
const ExcelContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

var ErrExportTooLarge = errors.New("report export exceeds row limit")

type ReportKind string

const (
	MovementReport  ReportKind = "attendance"
	DailyReport     ReportKind = "attendance-daily"
	LeaveReport     ReportKind = "leaves"
	ComplaintReport ReportKind = "complaints"
)

// Text is literal report content. Number is for numeric statistics;
// IDs, versions and timezone-aware timestamps remain text to preserve exact values.
type ExportCell struct {
	Text   string
	Number *int64
}
type ExportMetric struct {
	Name  string
	Value int64
}
type ExportDocument struct {
	Title    string
	Metadata [][2]string
	Summary  []ExportMetric
	Headers  []string
	Rows     [][]ExportCell
}
type ExportFile struct {
	Name string
	Data []byte
}
