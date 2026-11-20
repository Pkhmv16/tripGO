package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Pkhmv16/tripGO/internal/config"
	"github.com/Pkhmv16/tripGO/internal/container"
	"github.com/Pkhmv16/tripGO/internal/infra/telemetry"
)

func main() {
	if err := run(); err != nil {
		logger := telemetry.NewLogger(os.Stdout, slog.LevelError)
		logger.Error("service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := container.New(signalCtx, cfg)
	if err != nil {
		return fmt.Errorf("build application: %w", err)
	}

	return app.Run(signalCtx)
}
