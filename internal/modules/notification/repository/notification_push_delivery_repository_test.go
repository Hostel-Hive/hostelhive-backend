package repository

import (
	"testing"
	"time"
)

func TestPushRetryDelayBounds(t *testing.T) {
	for attempt := 1; attempt <= 8; attempt++ {
		minimum := min(time.Minute*time.Duration(1<<min(attempt-1, 6)), time.Hour)
		delay := retryDelay(attempt, 0)
		if delay < minimum || delay > minimum+minimum/10 {
			t.Fatal(attempt, delay)
		}
	}
	if retryDelay(1, 30*time.Minute) < 30*time.Minute {
		t.Fatal("provider backoff ignored")
	}
}
