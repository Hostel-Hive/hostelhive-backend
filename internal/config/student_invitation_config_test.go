package config

import (
	"reflect"
	"testing"
)

func TestStudentInvitationConfig(t *testing.T) {
	for _, tc := range []struct {
		enabled, domains string
		valid            bool
	}{
		{"false", "stu.kln.ac.lk", true}, {"true", "STU.KLN.AC.LK, students.example.edu", true},
		{"invalid", "stu.kln.ac.lk", false}, {"true", "", false}, {"true", "*", false}, {"true", "user@stu.kln.ac.lk", false},
		{"true", "-bad.example", false}, {"true", "bad-.example", false}, {"true", "bad..example", false},
	} {
		cfg, e := load(fromMap(map[string]string{"DATABASE_URL": "postgres://test:example@localhost/test", "FIREBASE_PROJECT_ID": "test", "STUDENT_INVITATIONS_ENABLED": tc.enabled, "STUDENT_EMAIL_DOMAINS": tc.domains}))
		if (e == nil) != tc.valid {
			t.Fatal(tc, e)
		}
		if e == nil && len(cfg.StudentEmailDomains) == 0 {
			t.Fatal("no domains")
		}
	}
}

func TestStudentInvitationDefaultAndOverriddenDomains(t *testing.T) {
	for _, tc := range []struct {
		name    string
		domains *string
		want    []string
	}{
		{name: "defaults", want: []string{"gmail.com", "stu.kln.ac.lk"}},
		{name: "normalized mixed domains", domains: domainSetting(" Gmail.COM,STU.KLN.AC.LK,gmail.com "), want: []string{"gmail.com", "stu.kln.ac.lk"}},
		{name: "explicit restriction preserved", domains: domainSetting("stu.kln.ac.lk"), want: []string{"stu.kln.ac.lk"}},
		{name: "other explicitly approved provider", domains: domainSetting("gmail.com,outlook.com,stu.kln.ac.lk"), want: []string{"gmail.com", "outlook.com", "stu.kln.ac.lk"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://test:example@localhost/test", "FIREBASE_PROJECT_ID": "test", "STUDENT_INVITATIONS_ENABLED": "true"}
			if tc.domains != nil {
				values["STUDENT_EMAIL_DOMAINS"] = *tc.domains
			}
			cfg, err := load(fromMap(values))
			if err != nil || !reflect.DeepEqual(cfg.StudentEmailDomains, tc.want) {
				t.Fatal("unexpected domain policy", cfg.StudentEmailDomains, err)
			}
		})
	}
}

func domainSetting(value string) *string { return &value }
