package workers

import (
	"context"
	"log/slog"
	"time"
)

type PushDelivery interface {
	Dispatch(context.Context) (int, error)
}

func RunPushDelivery(ctx context.Context, store PushDelivery, logger *slog.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		attempt, cancel := context.WithTimeout(ctx, 8*time.Second)
		n, err := store.Dispatch(attempt)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Warn("mobile push processing failed; pending delivery will retry")
		}
		if err == nil && n > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
