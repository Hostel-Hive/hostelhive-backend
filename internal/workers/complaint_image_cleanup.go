package workers

import (
	"context"
	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/service"
	"time"
)

func RunComplaintImageCleanup(ctx context.Context, repo s.ImageRepository, objects s.ImageObjects, timeout, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		for n := 0; n < 10 && ctx.Err() == nil; n++ {
			attempt, c := context.WithTimeout(ctx, timeout)
			worked, err := repo.CleanupImageOne(attempt, objects)
			c()
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
