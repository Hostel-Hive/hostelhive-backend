package handler

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	"time"
)

// One PDF API instance bounds concurrency across all four PDF routes.
func NewPDFExports(s service.ReportExports, timeout time.Duration) *ExportAPI {
	a := NewExports(s, timeout)
	a.contentType = domain.PDFContentType
	return a
}
