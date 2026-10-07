// Package server manages the HTTP server lifecycle and operational endpoints.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
	"github.com/Hostel-Hive/hostelhive-backend/internal/config"
	"github.com/Hostel-Hive/hostelhive-backend/internal/database"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseCheckTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on HTTP_ADDR: %w", err)
	}
	defer listener.Close()
	verifier, err := authentication.NewFirebaseVerifier(ctx, cfg.FirebaseProjectID)
	if err != nil {
		return err
	}
	protected := authentication.Middleware(verifier, authentication.NewPostgresAccounts(pool), cfg.AuthenticationTimeout)
	srv := &http.Server{
		Handler:           newHandler(pool.Ping, cfg.DatabaseCheckTimeout, protected),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
	logger.Info("HTTP server listening", "address", listener.Addr().String(), "environment", cfg.Environment)
	return serve(ctx, srv, listener, cfg)
}

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
