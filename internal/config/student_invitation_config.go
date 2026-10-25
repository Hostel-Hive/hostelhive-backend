package config

import (
	"fmt"
	"strconv"
	"strings"
)

func (c *Config) loadStudentInvitations(value func(string, string) string) error {
	enabled, e := strconv.ParseBool(value("STUDENT_INVITATIONS_ENABLED", "false"))
	if e != nil {
		return fmt.Errorf("STUDENT_INVITATIONS_ENABLED must be true or false")
	}
	c.StudentInvitationsEnabled = enabled
	seen := map[string]bool{}
	for _, domain := range strings.Split(value("STUDENT_EMAIL_DOMAINS", "gmail.com,stu.kln.ac.lk"), ",") {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if len(domain) > 253 || !strings.Contains(domain, ".") {
			return fmt.Errorf("STUDENT_EMAIL_DOMAINS must list valid email domains")
		}
		for _, label := range strings.Split(domain, ".") {
			if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return fmt.Errorf("STUDENT_EMAIL_DOMAINS must list valid email domains")
			}
			for _, r := range label {
				if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
					return fmt.Errorf("STUDENT_EMAIL_DOMAINS must list valid email domains")
				}
			}
		}
		if !seen[domain] {
			c.StudentEmailDomains = append(c.StudentEmailDomains, domain)
			seen[domain] = true
		}
	}
	return nil
}
