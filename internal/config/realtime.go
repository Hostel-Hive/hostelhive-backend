package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func (c *Config) loadRealtime(value func(string, string) string) error {
	enabled, err := strconv.ParseBool(value("RTDB_ENABLED", "false"))
	if err != nil {
		return fmt.Errorf("RTDB_ENABLED must be true or false")
	}
	c.RTDBEnabled = enabled
	if !enabled {
		return nil
	}
	c.FirebaseDatabaseURL = value("FIREBASE_DATABASE_URL", "")
	u, err := url.Parse(c.FirebaseDatabaseURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Port() != "" ||
		!(strings.HasSuffix(u.Hostname(), ".firebaseio.com") || strings.HasSuffix(u.Hostname(), ".firebasedatabase.app")) {
		return fmt.Errorf("FIREBASE_DATABASE_URL must be the HTTPS Firebase database root URL without credentials, query or fragment")
	}
	c.FirebaseDatabaseURL = strings.TrimRight(c.FirebaseDatabaseURL, "/")
	return nil
}
