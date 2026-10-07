package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/Hostel-Hive/hostelhive-backend/internal/config"
)

func serve(ctx context.Context, srv *http.Server, listener net.Listener, cfg config.Config) error {
	defer listener.Close()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// Force remaining connections closed after the grace period.
			_ = srv.Close()
			<-done
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		err := <-done
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}
}
