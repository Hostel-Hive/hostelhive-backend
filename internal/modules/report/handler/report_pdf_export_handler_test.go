package handler

import (
	"context"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type pdfStub struct{ excelStub }

func (s *pdfStub) Export(ctx context.Context, actor string, k domain.ReportKind, q dto.ExportQuery) (domain.ExportFile, error) {
	file, err := s.excelStub.Export(ctx, actor, k, q)
	file.Name = "report.pdf"
	if len(file.Data) > 0 {
		file.Data = []byte("%PDF-test")
	}
	return file, err
}

func TestPDFHTTPHeadersAndFailures(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{nil, 200}, {domain.ErrInvalid, 400}, {domain.ErrForbidden, 403}, {domain.ErrExportTooLarge, 413}, {domain.ErrUnavailable, 503}} {
		stub := &pdfStub{excelStub{err: tc.err}}
		api := NewPDFExports(stub, time.Second)
		for _, method := range []http.HandlerFunc{api.Movements, api.Daily, api.Leaves, api.Complaints} {
			r := httptest.NewRequest("GET", "/?from=2026-10-01&to=2026-10-15", nil)
			r.Header.Set("Authorization", "Bearer staff")
			w := httptest.NewRecorder()
			m.Middleware(verifier{}, accounts{identity.RoleAdmin}, time.Second)(method).ServeHTTP(w, r)
			if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(tc, w.Code, w.Body.String())
			}
			if tc.want == 200 {
				if w.Header().Get("Content-Type") != domain.PDFContentType || !strings.Contains(w.Header().Get("Content-Disposition"), "report.pdf") || w.Header().Get("Content-Length") != "9" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal(w.Header())
				}
			} else if w.Header().Get("Content-Disposition") != "" || w.Header().Get("Content-Type") == domain.PDFContentType {
				t.Fatal("PDF headers on error", w.Header())
			}
		}
	}
	// A PDF deadline must remain a JSON failure before download headers.
	api := NewPDFExports(blockingExport{make(chan struct{}, 1), make(chan struct{})}, time.Millisecond)
	r := httptest.NewRequest("GET", "/?from=2026-10-01&to=2026-10-15", nil)
	r.Header.Set("Authorization", "Bearer staff")
	w := httptest.NewRecorder()
	m.Middleware(verifier{}, accounts{identity.RoleAdmin}, time.Second)(http.HandlerFunc(api.Movements)).ServeHTTP(w, r)
	if w.Code != 503 || w.Header().Get("Content-Disposition") != "" {
		t.Fatal(w.Code, w.Body.String())
	}
}
