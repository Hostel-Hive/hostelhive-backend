package domain

import (
	"testing"
	"time"
)

func TestPermissionBounds(t *testing.T) {
	zone := time.FixedZone("Colombo", 19800)
	expected := time.Date(2027, 1, 1, 0, 10, 0, 0, zone)
	lower, upper := PermissionBounds(expected)
	if !lower.Equal(expected.Add(-30*time.Minute)) || !upper.Equal(expected.Add(30*time.Minute)) {
		t.Fatal(lower, upper)
	}
	for _, at := range []time.Time{lower, expected, upper} {
		lo, hi := PermissionBounds(at)
		if expected.Before(lo) || expected.After(hi) {
			t.Fatal("inclusive endpoint excluded", at)
		}
	}
	for _, at := range []time.Time{lower.Add(-time.Microsecond), upper.Add(time.Microsecond)} {
		lo, hi := PermissionBounds(at)
		if !expected.Before(lo) && !expected.After(hi) {
			t.Fatal("outside window accepted", at)
		}
	}
	if lower.Day() != 31 || lower.Month() != time.December {
		t.Fatal("window must cross midnight")
	}
}
