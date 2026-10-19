package workers

import (
	"context"
	"log/slog"
	"time"
)

type StudentInvitationDelivery interface {
	Dispatch(context.Context) (bool, error)
}

func RunStudentInvitations(ctx context.Context, delivery StudentInvitationDelivery, logger *slog.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		attempt, cancel := context.WithTimeout(ctx, 8*time.Second)
		worked, e := delivery.Dispatch(attempt)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if e != nil {
			logger.Warn("Student invitation processing failed; pending work will retry")
		}
		if e == nil && worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
