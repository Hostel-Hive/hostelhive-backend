package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
)

type stubReports struct{ err error }

func (s stubReports) Movements(context.Context, string, dto.AttendanceQuery) (domain.MovementPage, error) {
	return domain.MovementPage{}, s.err
}
func (s stubReports) Daily(context.Context, string, dto.AttendanceQuery) (domain.DailyPage, error) {
	return domain.DailyPage{}, s.err
}

type verifier struct{}

func (verifier) Verify(context.Context, string) (string, error) { return "staff", nil }

type accounts struct{ role string }

func (a accounts) FindByFirebaseUID(context.Context, string) (identity.Account, error) {
	return identity.Account{FirebaseUID: "staff", Role: a.role, IsActive: true}, nil
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (brokenBody) Close() error             { return nil }
func TestReportFailureMappingAndDirectAuthority(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{domain.ErrInvalid, 400}, {domain.ErrForbidden, 403}, {domain.ErrUnavailable, 503}} {
		api := NewAttendance(stubReports{tc.err}, time.Second)
		for _, method := range []http.HandlerFunc{api.Movements, api.Daily} {
			r := httptest.NewRequest("GET", "/?from=2026-10-14&to=2026-10-15", nil)
			w := httptest.NewRecorder()
			method(w, r)
			if w.Code != 401 {
				t.Fatal("missing account", w.Code)
			}
			r.Header.Set("Authorization", "Bearer staff")
			w = httptest.NewRecorder()
			m.Middleware(verifier{}, accounts{identity.RoleAdmin}, time.Second)(method).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(tc, w.Code)
			}
			w = httptest.NewRecorder()
			m.Middleware(verifier{}, accounts{identity.RoleStudent}, time.Second)(method).ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatal("direct role check", w.Code)
			}
			r.Body = brokenBody{}
			w = httptest.NewRecorder()
			m.Middleware(verifier{}, accounts{identity.RoleAdmin}, time.Second)(method).ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatal("broken body", w.Code)
			}
		}
	}
}
