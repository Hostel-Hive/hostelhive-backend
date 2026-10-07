package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Hostel-Hive/hostelhive-backend/internal/config"
	identityhandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/handler"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/repository"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/identity/service"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/student"
	shandler "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/handler"
	srepo "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/repository"
	sservice "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	platformfirebase "github.com/Hostel-Hive/hostelhive-backend/internal/platform/firebase"
	database "github.com/Hostel-Hive/hostelhive-backend/internal/platform/postgres"
	authentication "github.com/Hostel-Hive/hostelhive-backend/internal/shared/middleware"
	"github.com/Hostel-Hive/hostelhive-backend/internal/workers"
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
	firebaseClient, err := platformfirebase.NewFirebaseClient(ctx, cfg.FirebaseProjectID)
	if err != nil {
		return err
	}
	protected := authentication.Middleware(platformfirebase.NewVerifier(firebaseClient), repository.NewPostgresAccounts(pool), cfg.AuthenticationTimeout)
	userService := service.NewProvisioningService(repository.NewPostgresRepository(pool), platformfirebase.NewFirebaseIdentities(firebaseClient))
	managementStore := repository.NewStore(pool)
	revoker := platformfirebase.NewFirebaseRevoker(firebaseClient)
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		workers.RunUserRevocations(workerCtx, managementStore, revoker, cfg.AccountManagementTimeout, 30*time.Second)
	}()
	defer func() { stopWorker(); <-workerDone }()
	handler := newAPIHandler(pool.Ping, cfg.DatabaseCheckTimeout, protected, identityhandler.Handler(userService, cfg.ProvisioningTimeout), identityhandler.NewAPI(service.NewAccounts(managementStore, revoker), cfg.AccountManagementTimeout))
	student.Register(shandler.NewAPI(sservice.New(srepo.NewStore(pool)), cfg.StudentProfileTimeout), handler, protected)
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
