package config

import "testing"

func TestRealtimeConfiguration(t *testing.T) {
	value := func(values map[string]string) func(string, string) string {
		return func(key, fallback string) string {
			if v, ok := values[key]; ok {
				return v
			}
			return fallback
		}
	}
	var cfg Config
	if err := cfg.loadRealtime(value(nil)); err != nil || cfg.RTDBEnabled {
		t.Fatal(cfg, err)
	}
	for _, url := range []string{"", "http://x.firebaseio.com", "https://x.firebaseio.com.evil.test", "https://user:secret@x.firebaseio.com", "https://x.firebaseio.com/path", "https://x.firebaseio.com?key=secret", "https://x.firebaseio.com:443", "https://localhost"} {
		if cfg.loadRealtime(value(map[string]string{"RTDB_ENABLED": "true", "FIREBASE_DATABASE_URL": url})) == nil {
			t.Fatalf("accepted invalid URL: %s", url)
		}
	}
	for _, url := range []string{"https://hostel-hive-152ef-default-rtdb.firebaseio.com/", "https://hostel-hive-152ef-default-rtdb.asia-southeast1.firebasedatabase.app"} {
		if err := cfg.loadRealtime(value(map[string]string{"RTDB_ENABLED": "true", "FIREBASE_DATABASE_URL": url})); err != nil || !cfg.RTDBEnabled {
			t.Fatal(err)
		}
	}
	if cfg.loadRealtime(value(map[string]string{"RTDB_ENABLED": "bad"})) == nil {
		t.Fatal("accepted invalid boolean")
	}
}
