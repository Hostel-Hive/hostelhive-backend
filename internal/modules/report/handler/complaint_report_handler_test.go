package handler

import (
	"context"
	identity "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/dto"
	m "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type complaintStub struct{ err error }

func (s complaintStub) Records(context.Context, string, dto.ComplaintQuery) (domain.ComplaintPage, error) {
	return domain.ComplaintPage{}, s.err
}
func TestComplaintReportFailureMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{domain.ErrInvalid, 400}, {domain.ErrForbidden, 403}, {domain.ErrUnavailable, 503}} {
		api := NewComplaints(complaintStub{tc.err}, time.Second)
		r := httptest.NewRequest("GET", "/?from=2026-10-14&to=2026-10-15", nil)
		w := httptest.NewRecorder()
		api.Records(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
		r.Header.Set("Authorization", "Bearer staff")
		w = httptest.NewRecorder()
		m.Middleware(verifier{}, accounts{identity.RoleAdmin}, time.Second)(http.HandlerFunc(api.Records)).ServeHTTP(w, r)
		if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(tc, w.Code)
		}
		w = httptest.NewRecorder()
		m.Middleware(verifier{}, accounts{identity.RoleStudent}, time.Second)(http.HandlerFunc(api.Records)).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
		r.Body = brokenBody{}
		w = httptest.NewRecorder()
		m.Middleware(verifier{}, accounts{identity.RoleAdmin}, time.Second)(http.HandlerFunc(api.Records)).ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}
