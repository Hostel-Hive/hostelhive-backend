package config

import "testing"

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
