package config

import (
	"fmt"
	"time"
	_ "time/tzdata" // IANA zones are also available in minimal containers and Windows.
)

func (c *Config) loadLateAccess(value func(string, string) string) error {
	zoneName := value("HOSTEL_TIMEZONE", "Asia/Colombo")
	if zoneName == "" || zoneName == "Local" {
		return fmt.Errorf("HOSTEL_TIMEZONE must be an explicit IANA timezone")
	}
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		return fmt.Errorf("HOSTEL_TIMEZONE must be a valid IANA timezone")
	}
	c.HostelLocation = zone
	for _, setting := range []struct {
		key, fallback string
		target        *int
	}{
		{"HOSTEL_EXIT_OPENING", "06:00", &c.HostelExitOpening},
		{"HOSTEL_ENTRY_DEADLINE", "20:00", &c.HostelEntryDeadline},
	} {
		raw := value(setting.key, setting.fallback)
		parsed, err := time.Parse("15:04", raw)
		if err != nil || parsed.Format("15:04") != raw {
			return fmt.Errorf("%s must use HH:MM", setting.key)
		}
		*setting.target = parsed.Hour()*60 + parsed.Minute()
	}
	if c.HostelExitOpening >= c.HostelEntryDeadline {
		return fmt.Errorf("HOSTEL_EXIT_OPENING must precede HOSTEL_ENTRY_DEADLINE")
	}
	return nil
}
