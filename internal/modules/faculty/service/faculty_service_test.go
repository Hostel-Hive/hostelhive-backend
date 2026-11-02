package service

import (
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/dto"
	"strings"
	"testing"
)

func TestFacultyNames(t *testing.T) {
	for _, name := range []string{"", "  ", "Science\nFaculty", strings.Repeat("x", 121), string([]byte{0xff})} {
		if _, e := normalize(dto.Details{Name: name}); e == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
	}
	v, e := normalize(dto.Details{Name: "  Science  "})
	if e != nil || v.Name != "Science" {
		t.Fatal(v, e)
	}
}
