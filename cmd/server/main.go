package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Hostel-Hive/hostelhive-backend/internal/config"
	"github.com/Hostel-Hive/hostelhive-backend/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code := run(ctx, logger)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, logger *slog.Logger) int {
	return runWithServer(ctx, logger, server.Run)
}

func runWithServer(ctx context.Context, logger *slog.Logger, start func(context.Context, config.Config, *slog.Logger) error) int {
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		return 1
	}
	if err := start(ctx, cfg, logger); err != nil {
		logger.Error("server stopped with an error", "error", err)
		return 1
	}
	logger.Info("server stopped gracefully")
	return 0
}
