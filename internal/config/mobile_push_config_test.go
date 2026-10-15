package config

import "testing"

func TestFCMConfiguration(t *testing.T) {
	for _, tc := range []struct {
		v           string
		ok, enabled bool
	}{{"true", true, true}, {"false", true, false}, {"", false, false}, {"invalid", false, false}} {
		cfg, err := load(fromMap(map[string]string{"DATABASE_URL": "postgres://test:example@localhost/test", "FIREBASE_PROJECT_ID": "test", "FCM_ENABLED": tc.v}))
		if (err == nil) != tc.ok || (err == nil && cfg.FCMEnabled != tc.enabled) {
			t.Fatal(tc, cfg.FCMEnabled, err)
		}
	}
}
