package handler

import (
	"context"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/service"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type excelStub struct {
	err   error
	empty bool
	calls int
	kind  domain.ReportKind
	query dto.ExportQuery
}

func (s *excelStub) Export(_ context.Context, _ string, k domain.ReportKind, q dto.ExportQuery) (domain.ExportFile, error) {
	s.calls++
	s.kind, s.query = k, q
	file := domain.ExportFile{Name: "report.xlsx"}
	if !s.empty {
		file.Data = []byte("PK-workbook")
	}
	return file, s.err
}
func TestExportHTTPHeadersErrorsAndParsing(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{nil, 200}, {domain.ErrInvalid, 400}, {domain.ErrForbidden, 403}, {domain.ErrExportTooLarge, 413}, {domain.ErrUnavailable, 503}} {
		stub := &excelStub{err: tc.err}
		api := NewExports(stub, time.Second)
		for _, method := range []http.HandlerFunc{api.Movements, api.Daily, api.Leaves, api.Complaints} {
			r := httptest.NewRequest("GET", "/?from=2026-10-14&to=2026-10-15", nil)
			r.Header.Set("Authorization", "Bearer staff")
			w := httptest.NewRecorder()
			m.Middleware(verifier{}, accounts{identity.RoleAdmin}, time.Second)(method).ServeHTTP(w, r)
			if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(tc, w.Code, w.Body.String())
			}
			if tc.want == 200 {
				if w.Header().Get("Content-Type") != domain.ExcelContentType || !strings.Contains(w.Header().Get("Content-Disposition"), "report.xlsx") || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Length") != "11" {
					t.Fatal(w.Header())
				}
			} else if w.Header().Get("Content-Disposition") != "" || w.Header().Get("Content-Type") == domain.ExcelContentType {
				t.Fatal("binary headers on error")
			}
		}
	}
	stub := &excelStub{}
	api := NewExports(stub, time.Second)
	call := func(path, role, body string, want int) {
		t.Helper()
		r := httptest.NewRequest("GET", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer staff")
		w := httptest.NewRecorder()
		m.Middleware(verifier{}, accounts{role}, time.Second)(http.HandlerFunc(api.Complaints)).ServeHTTP(w, r)
		if w.Code != want {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	q := "/?from=2026-10-14&to=2026-10-15"
	for _, bad := range []string{"&limit=1", "&offset=0", "&direction=IN", "&unknown=1", "&category=", "&status=pending&status=resolved"} {
		call(q+bad, identity.RoleAdmin, "", 400)
	}
	call(q, identity.RoleAdmin, "{}", 400)
	call(q, identity.RoleStudent, "", 403)
	if stub.calls != 0 {
		t.Fatal("invalid/forbidden request reached service")
	}
	call(q+"&category=maintenance&status=pending", identity.RoleWarden, "", 200)
	if stub.kind != domain.ComplaintReport || stub.query.Category != "maintenance" || stub.query.Status != "pending" {
		t.Fatal(stub)
	}
	r := httptest.NewRequest("GET", q, nil)
	w := httptest.NewRecorder()
	api.Complaints(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	stub.empty = true
	call(q, identity.RoleAdmin, "", 503)
}

type blockingExport struct {
	entered chan struct{}
	release chan struct{}
}

func (s blockingExport) Export(ctx context.Context, _ string, _ domain.ReportKind, _ dto.ExportQuery) (domain.ExportFile, error) {
	s.entered <- struct{}{}
	select {
	case <-s.release:
		return domain.ExportFile{Name: "ready.xlsx", Data: []byte("PK")}, nil
	case <-ctx.Done():
		return domain.ExportFile{}, ctx.Err()
	}
}
func TestExportConcurrencyBound(t *testing.T)    { testExportConcurrencyBound(t, NewExports) }
func TestPDFExportConcurrencyBound(t *testing.T) { testExportConcurrencyBound(t, NewPDFExports) }
func testExportConcurrencyBound(t *testing.T, constructor func(service.ReportExports, time.Duration) *ExportAPI) {
	stub := blockingExport{make(chan struct{}, 2), make(chan struct{})}
	api := constructor(stub, 5*time.Second)
	method := m.Middleware(verifier{}, accounts{identity.RoleAdmin}, time.Second)(http.HandlerFunc(api.Movements))
	call := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/?from=2026-10-14&to=2026-10-15", nil)
		r.Header.Set("Authorization", "Bearer staff")
		w := httptest.NewRecorder()
		method.ServeHTTP(w, r)
		return w
	}
	var wg sync.WaitGroup
	var once sync.Once
	results := make(chan int, 2)
	release := func() { once.Do(func() { close(stub.release) }); wg.Wait() }
	defer release()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- call().Code }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-stub.entered:
		case <-time.After(3 * time.Second):
			t.Fatal("exports did not enter service")
		}
	}
	if w := call(); w.Code != 429 || !strings.Contains(w.Body.String(), "report_export_busy") {
		t.Fatal(w.Code, w.Body.String())
	}
	release()
	for i := 0; i < 2; i++ {
		if code := <-results; code != 200 {
			t.Fatal(code)
		}
	}
}
