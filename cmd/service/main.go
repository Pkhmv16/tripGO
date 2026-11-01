package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Pkhmv16/tripGO/internal/config"
	"github.com/Pkhmv16/tripGO/internal/container"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
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

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- app.Run()
	}()

	var runErr error
	select {
	case <-signalCtx.Done():
	case runErr = <-serverErrors:
	}
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return errors.Join(runErr, app.Shutdown(shutdownCtx))
}
