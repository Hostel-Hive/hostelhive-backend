// Package server manages the HTTP server lifecycle and operational endpoints.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/accountmanagement"
	"github.com/Hostel-Hive/hostelhive-backend/internal/authentication"
	"github.com/Hostel-Hive/hostelhive-backend/internal/config"
	"github.com/Hostel-Hive/hostelhive-backend/internal/database"
	"github.com/Hostel-Hive/hostelhive-backend/internal/provisioning"
	"github.com/Hostel-Hive/hostelhive-backend/internal/students"
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
	firebaseClient, err := authentication.NewFirebaseClient(ctx, cfg.FirebaseProjectID)
	if err != nil {
		return err
	}
	protected := authentication.Middleware(authentication.NewVerifier(firebaseClient), authentication.NewPostgresAccounts(pool), cfg.AuthenticationTimeout)
	userService := provisioning.NewService(provisioning.NewPostgresRepository(pool), provisioning.NewFirebaseIdentities(firebaseClient))
	managementStore := accountmanagement.NewStore(pool)
	revoker := accountmanagement.NewFirebaseRevoker(firebaseClient)
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		managementStore.RunRetries(workerCtx, revoker, cfg.AccountManagementTimeout, 30*time.Second)
	}()
	defer func() { stopWorker(); <-workerDone }()
	handler := newAPIHandler(pool.Ping, cfg.DatabaseCheckTimeout, protected, provisioning.Handler(userService, cfg.ProvisioningTimeout), accountmanagement.NewAPI(managementStore, revoker, cfg.AccountManagementTimeout))
	students.NewAPI(students.NewStore(pool), cfg.StudentProfileTimeout).Register(handler, protected)
	srv := &http.Server{
		Handler:           handler,
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
