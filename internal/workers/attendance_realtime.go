package workers

import (
	"context"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/attendance/service"
	"log/slog"
	"time"
)

func RunAttendanceRealtime(ctx context.Context, repo service.RealtimeRepository, publisher service.RealtimePublisher, logger *slog.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := repo.Reconcile(attempt)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.Warn("attendance realtime reconciliation failed")
		}
		// Revoke/expire grants before publishing the next shared update.
		for n := 0; n < 50 && ctx.Err() == nil; n++ {
			attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
			worked, err := repo.SyncGrant(attempt, publisher, "")
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Warn("attendance realtime grant retry pending")
			}
			if !worked || err != nil {
				break
			}
		}
		attempt, cancel = context.WithTimeout(ctx, 5*time.Second)
		_, err = repo.PublishOne(attempt, publisher)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.Warn("attendance realtime publication retry pending")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
