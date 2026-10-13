package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	domain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	studenthandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	service "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type fakeImporter struct {
	calls int
	actor string
	raw   []byte
	err   error
}

func (s *fakeImporter) Import(ctx context.Context, actor string, raw []byte) (domain.ImportReport, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	s.calls++
	s.actor = actor
	s.raw = raw
	return domain.ImportReport{Total: 1, Created: 1, Rows: []domain.ImportRowResult{{Record: 1, Line: 2, Status: "created", StudentID: testID}}}, s.err
}
func importHandler(s *fakeImporter, role string, active bool) http.Handler {
	mux := http.NewServeMux()
	student.RegisterImport(studenthandler.NewImportAPI(s, time.Second), mux, authentication.Middleware(verifier{}, accounts{role, active}, time.Second))
	return mux
}
func importRequest(h http.Handler, method, path, body, media string, token bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token {
		req.Header.Set("Authorization", "Bearer token")
	}
	req.Header.Set("Content-Type", media)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
func TestImportRoutePolicy(t *testing.T) {
	for _, role := range []string{identity.RoleAdmin, identity.RoleWarden, identity.RoleStudent, identity.RoleSubWarden, identity.RoleSecurityStaff} {
		t.Run(role, func(t *testing.T) {
			s := &fakeImporter{}
			h := importHandler(s, role, true)
			for _, route := range []struct{ method, path string }{{"POST", "/api/v1/students/import"}, {"GET", "/api/v1/students/import/template"}} {
				w := importRequest(h, route.method, route.path, "csv", "text/csv", true)
				expected := 403
				if role == identity.RoleAdmin {
					expected = 200
				}
				if w.Code != expected {
					t.Fatalf("%s %d", route.path, w.Code)
				}
			}
			if role != identity.RoleAdmin && s.calls != 0 {
				t.Fatal("unauthorized importer reached")
			}
		})
	}
	anonymous := &fakeImporter{}
	w := importRequest(importHandler(anonymous, identity.RoleAdmin, true), "POST", "/api/v1/students/import", "csv", "text/csv", false)
	if w.Code != 401 || anonymous.calls != 0 {
		t.Fatal("missing token not denied")
	}
	s := &fakeImporter{}
	w = importRequest(importHandler(s, identity.RoleAdmin, false), "POST", "/api/v1/students/import", "csv", "text/csv", true)
	if w.Code != 403 || s.calls != 0 {
		t.Fatal("inactive admin allowed")
	}
}
func TestImportTransportAndSafeResults(t *testing.T) {
	for _, media := range []string{"", "application/json", "text/csv; charset=utf-16", "multipart/form-data"} {
		s := &fakeImporter{}
		w := importRequest(importHandler(s, identity.RoleAdmin, true), "POST", "/api/v1/students/import", "csv", media, true)
		if w.Code != 415 || s.calls != 0 {
			t.Fatalf("media %s %d", media, w.Code)
		}
	}
	s := &fakeImporter{}
	h := importHandler(s, identity.RoleAdmin, true)
	w := importRequest(h, "POST", "/api/v1/students/import", strings.Repeat("x", service.MaxImportBytes+1), "text/csv", true)
	if w.Code != 413 || s.calls != 0 {
		t.Fatal("size limit")
	}
	w = importRequest(h, "POST", "/api/v1/students/import?role=admin", "csv", "text/csv", true)
	if w.Code != 400 || s.calls != 0 {
		t.Fatal("query accepted")
	}
	w = importRequest(h, "POST", "/api/v1/students/import", "csv", "text/csv; charset=UTF-8", true)
	if w.Code != 200 || s.actor != "verified-admin" || string(s.raw) != "csv" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("success %d", w.Code)
	}
	s.err = domain.ErrInvalid
	w = importRequest(h, "POST", "/api/v1/students/import", "bad", "text/csv", true)
	if w.Code != 400 {
		t.Fatal("format error")
	}
	var errorBody map[string]string
	if json.Unmarshal(w.Body.Bytes(), &errorBody) != nil || errorBody["error"] != "invalid_csv" {
		t.Fatal("safe structural error")
	}
	for _, test := range []struct {
		err  error
		code int
	}{{domain.ErrForbidden, 403}, {domain.ErrUnavailable, 503}} {
		s.err = test.err
		failureResponse := importRequest(h, "POST", "/api/v1/students/import", "csv", "text/csv", true)
		if failureResponse.Code != test.code {
			t.Fatalf("service error %d", failureResponse.Code)
		}
	}

	w = importRequest(h, "GET", "/api/v1/students/import/template", "", "", true)
	if w.Code != 200 || w.Body.String() != service.CSVHeader+"\r\n" || !strings.Contains(w.Header().Get("Content-Disposition"), "student-import-template.csv") {
		t.Fatal("template mismatch")
	}
}
