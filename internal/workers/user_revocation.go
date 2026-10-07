package workers

import (
	"context"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
)

func RunUserRevocations(ctx context.Context, s service.AccountRepository, r service.Revoker, timeout, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		for n := 0; n < 10 && ctx.Err() == nil; n++ {
			attempt, cancel := context.WithTimeout(ctx, timeout)
			worked, err := s.RetryOne(attempt, r, "")
			cancel()
			if !worked || err != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
