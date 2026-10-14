package workers

import (
	"context"
	"log/slog"
	"time"
)

type NotificationDelivery interface {
	Deliver(context.Context) (int, error)
}

func RunNotificationDelivery(ctx context.Context, store NotificationDelivery, logger *slog.Logger) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		attempt, cancel := context.WithTimeout(ctx, 8*time.Second)
		n, e := store.Deliver(attempt)
		cancel()
		if e != nil && ctx.Err() == nil {
			logger.Warn("notification inbox delivery failed; pending events will retry")
		}
		if ctx.Err() != nil {
			return
		}
		if e == nil && n == 100 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
