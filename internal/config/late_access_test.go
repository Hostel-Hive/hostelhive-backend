package config

import (
	"testing"
)

func TestLateAccessConfiguration(t *testing.T) {
	c := Config{}
	if e := c.loadLateAccess(func(_, fallback string) string { return fallback }); e != nil || c.HostelLocation.String() != "Asia/Colombo" || c.HostelExitOpening != 360 || c.HostelEntryDeadline != 1200 {
		t.Fatal(c, e)
	}
	for _, values := range []map[string]string{
		{"HOSTEL_TIMEZONE": "invalid/zone"}, {"HOSTEL_TIMEZONE": ""}, {"HOSTEL_EXIT_OPENING": "6:00"}, {"HOSTEL_ENTRY_DEADLINE": "24:00"}, {"HOSTEL_EXIT_OPENING": "20:00"}, {"HOSTEL_ENTRY_DEADLINE": "05:00"},
	} {
		e := c.loadLateAccess(func(k, f string) string {
			if v, ok := values[k]; ok {
				return v
			}
			return f
		})
		if e == nil {
			t.Fatalf("accepted invalid configuration %v", values)
		}
	}
	if e := c.loadLateAccess(func(k, f string) string {
		if k == "HOSTEL_EXIT_OPENING" {
			return "07:00"
		}
		return f
	}); e != nil || c.HostelExitOpening != 420 {
		t.Fatal(c, e)
	}
}
