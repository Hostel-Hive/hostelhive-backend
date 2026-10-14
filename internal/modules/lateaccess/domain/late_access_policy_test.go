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
		{"OUT", "05:59:59", true}, {"OUT", "06:00:00", false}, {"OUT", "20:00:00", false},
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
}
