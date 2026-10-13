package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/domain"
)

func TestScanJSONRejectsAmbiguousOrOversizedRequests(t *testing.T) {
	for _, tc := range []struct {
		body, media, query string
		status             int
	}{
		{`{"extra":true}`, "application/json", "", 400},
		{`{} {}`, "application/json", "", 400},
		{"{", "application/json", "", 400},
		{string([]byte{0xff}), "application/json", "", 400},
		{strings.Repeat("x", 2049), "application/json", "", 413},
		{`{}`, "text/plain", "", 415},
		{`{}`, "application/json", "?offline=true", 400},
	} {
		r := httptest.NewRequest("POST", "/api/v1/device/scans"+tc.query, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.media)
		w := httptest.NewRecorder()
		var in struct{}
		if scanJSON(w, r, &in) || w.Code != tc.status {
			t.Fatalf("status %d expected %d", w.Code, tc.status)
		}
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	w := httptest.NewRecorder()
	var in struct{}
	if !scanJSON(w, r, &in) {
		t.Fatal("valid JSON rejected")
	}
	for _, err := range []error{domain.ErrDirection, domain.ErrStudent, domain.ErrEventConflict, domain.ErrInvalid, domain.ErrUnauthorized, domain.ErrForbidden, domain.ErrNotFound, domain.ErrUnavailable} {
		w := httptest.NewRecorder()
		scanFailure(w, err)
		if w.Code < 400 {
			t.Fatal("error response")
		}
	}
	// Direct invocation cannot bypass device middleware.
	api := NewScanAPI(nil, 0)
	w = httptest.NewRecorder()
	api.Record(w, r)
	if w.Code != 401 {
		t.Fatal("missing device accepted")
	}
}
