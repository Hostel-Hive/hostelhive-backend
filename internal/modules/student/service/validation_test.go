package service

import (
	"errors"
	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	sdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"strings"
	"testing"
)

func validDetails() sdto.Details {
	return sdto.Details{IndexNo: "SC/2026/001", FullName: "Test Student", Faculty: "Science", Year: 1, ContactPhone: "+94 771234567", Guardians: []sdomain.GuardianInput{{Name: "Test Guardian", Relationship: "Parent", ContactPhone: "0771234567"}}}
}

func TestProfileValidation(t *testing.T) {
	for _, change := range []func(*sdto.Details){func(v *sdto.Details) { v.IndexNo = "" }, func(v *sdto.Details) { v.IndexNo = strings.Repeat("x", 65) }, func(v *sdto.Details) { v.FullName = "\n" }, func(v *sdto.Details) { v.FullName = string([]byte{0xff}) }, func(v *sdto.Details) { v.Faculty = "" }, func(v *sdto.Details) { v.Year = 0 }, func(v *sdto.Details) { v.Year = 11 }, func(v *sdto.Details) { v.ContactPhone = "-------" }, func(v *sdto.Details) { v.ContactPhone = "abc1234567" }, func(v *sdto.Details) { v.Guardians = nil }, func(v *sdto.Details) { v.Guardians[0].Name = "" }, func(v *sdto.Details) { v.Guardians[0].Relationship = "" }, func(v *sdto.Details) { v.Guardians[0].ContactPhone = "123" }} {
		v := validDetails()
		change(&v)
		if _, err := Normalize(v); !errors.Is(err, sdomain.ErrInvalid) {
			t.Fatal("accepted invalid profile")
		}
	}
	v := validDetails()
	v.FullName = "  \u0dc3\u0dc0\u0dd2 Student  "
	normalized, err := Normalize(v)
	if err != nil || strings.HasPrefix(normalized.FullName, " ") {
		t.Fatal("Unicode or trimming failed")
	}
	v = validDetails()
	v.IndexNo = strings.Repeat("x", 64)
	v.FullName = strings.Repeat("\u754c", 200)
	v.Faculty = strings.Repeat("x", 120)
	v.Year = 10
	if _, err = Normalize(v); err != nil {
		t.Fatal("valid boundaries rejected")
	}
}
