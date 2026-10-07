package workers

import (
	"context"
	"time"

	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
)

func RunStudentImageCleanup(ctx context.Context, repo s.ImageRepository, objects s.Objects, timeout, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		for n := 0; n < 10 && ctx.Err() == nil; n++ {
			attempt, cancel := context.WithTimeout(ctx, timeout)
			worked, err := repo.CleanupOne(attempt, objects)
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
