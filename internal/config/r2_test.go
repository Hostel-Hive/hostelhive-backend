package config

import (
	"strings"
	"testing"
)

func TestR2Configuration(t *testing.T) {
	base := map[string]string{"FIREBASE_PROJECT_ID": "test", "DATABASE_URL": "postgres://test:password@localhost/test", "R2_ENABLED": "true", "R2_ACCOUNT_ID": strings.Repeat("a", 32), "R2_BUCKET": "private-images", "R2_ACCESS_KEY_ID": "test-access", "R2_SECRET_ACCESS_KEY": "private-test-secret"}
	for _, tc := range []struct{ key, value string }{{"R2_ENABLED", "bad"}, {"R2_ACCOUNT_ID", "bad"}, {"R2_ACCOUNT_ID", "https://untrusted.invalid"}, {"R2_BUCKET", "a"}, {"R2_BUCKET", "../bucket"}, {"R2_ACCESS_KEY_ID", ""}, {"R2_SECRET_ACCESS_KEY", "secret with spaces"}} {
		v := map[string]string{}
		for k, x := range base {
			v[k] = x
		}
		v[tc.key] = tc.value
		_, err := load(fromMap(v))
		if err == nil || !strings.Contains(err.Error(), tc.key) || strings.Contains(err.Error(), base["R2_SECRET_ACCESS_KEY"]) {
			t.Fatalf("bad validation for %s: %v", tc.key, err)
		}
	}
	cfg, err := load(fromMap(base))
	if err != nil || !cfg.R2Enabled {
		t.Fatal("valid configuration", err)
	}
	base["R2_ENABLED"] = "false"
	delete(base, "R2_ACCOUNT_ID")
	cfg, err = load(fromMap(base))
	if err != nil || cfg.R2Enabled || cfg.R2SecretAccessKey != "" {
		t.Fatal("disabled storage loads credentials", err)
	}
}
