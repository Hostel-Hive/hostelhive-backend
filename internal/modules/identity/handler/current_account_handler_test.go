package handler

import (
	"net/http/httptest"
	"testing"
)

func TestMeRequiresMiddleware(t *testing.T) {
	w := httptest.NewRecorder()
	Me(w, httptest.NewRequest("GET", "/api/v1/me", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
