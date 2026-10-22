package domain

import (
	"testing"
	"time"
)

func TestPermissionBoundaries(t *testing.T) {
	zone := time.FixedZone("Colombo", 19800)
	policy := Policy{Location: zone, ExitOpening: 360, EntryDeadline: 1200}
	for _, v := range []struct {
		direction, local string
		want             bool
	}{
		{"OUT", "00:00:00", true}, {"OUT", "05:59:59", true}, {"OUT", "06:00:00", false}, {"OUT", "19:59:59", false}, {"OUT", "20:00:00", true}, {"OUT", "23:59:59", true},
		{"IN", "19:59:59", false}, {"IN", "20:00:00", true}, {"IN", "23:59:59", true}, {"IN", "00:00:00", true}, {"IN", "05:59:59", true}, {"IN", "06:00:00", false}, {"bad", "21:00:00", false},
	} {
		at, err := time.ParseInLocation("2006-01-02 15:04:05", "2027-01-01 "+v.local, zone)
		if err != nil {
			t.Fatal(err)
		}
		if got := policy.RequiresPermission(v.direction, at.UTC()); got != v.want {
			t.Errorf("%s %s: %v", v.direction, v.local, got)
		}
	}
	policy.ExitOpening = 420
	policy.EntryDeadline = 1260
	at := time.Date(2027, 1, 1, 20, 30, 0, 0, zone)
	if policy.RequiresPermission("IN", at) || !policy.RequiresPermission("OUT", at.Add(-14*time.Hour)) {
		t.Fatal("custom hours ignored")
	}
	if policy.RequiresPermission("OUT", at) || !policy.RequiresPermission("OUT", at.Add(time.Hour)) {
		t.Fatal("custom exit closing ignored")
	}
}

func TestExitIncidentTypes(t *testing.T) {
	zone := time.FixedZone("Colombo", 19800)
	policy := Policy{Location: zone, ExitOpening: 360, EntryDeadline: 1200}
	for _, tc := range []struct {
		hour int
		want string
	}{{0, "early_exit"}, {5, "early_exit"}, {20, "late_exit"}, {23, "late_exit"}} {
		at := time.Date(2026, 10, 22, tc.hour, 0, 0, 0, zone)
		if got := policy.ExitIncidentType(at.UTC()); got != tc.want {
			t.Fatalf("hour %d: got %s want %s", tc.hour, got, tc.want)
		}
	}
}
