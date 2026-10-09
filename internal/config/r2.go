package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func (c *Config) loadR2(value func(string, string) string) error {
	enabled, err := strconv.ParseBool(value("R2_ENABLED", "false"))
	if err != nil {
		return fmt.Errorf("R2_ENABLED must be true or false")
	}
	c.R2Enabled = enabled
	if !enabled {
		return nil
	}
	c.R2AccountID = value("R2_ACCOUNT_ID", "")
	c.R2Bucket = value("R2_BUCKET", "")
	c.R2AccessKeyID = value("R2_ACCESS_KEY_ID", "")
	c.R2SecretAccessKey = value("R2_SECRET_ACCESS_KEY", "")
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(c.R2AccountID) {
		return fmt.Errorf("R2_ACCOUNT_ID must be a 32-character lowercase hexadecimal account ID")
	}
	if len(c.R2Bucket) < 3 || len(c.R2Bucket) > 63 || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`).MatchString(c.R2Bucket) {
		return fmt.Errorf("R2_BUCKET must be a 3-63 character lowercase bucket name")
	}
	for key, v := range map[string]string{"R2_ACCESS_KEY_ID": c.R2AccessKeyID, "R2_SECRET_ACCESS_KEY": c.R2SecretAccessKey} {
		if v == "" || strings.ContainsAny(v, " \t\r\n") {
			return fmt.Errorf("%s must be nonempty without whitespace", key)
		}
	}
	return nil
}
