package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Pkhmv16/tripGO/internal/config"
	"github.com/Pkhmv16/tripGO/internal/handler"
	"github.com/Pkhmv16/tripGO/internal/infrastructure"
	"github.com/Pkhmv16/tripGO/internal/infrastructure/repository"
	"github.com/Pkhmv16/tripGO/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
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

	pool, err := infrastructure.NewPostgresPool(signalCtx,
		infrastructure.PostgresConfig{
			URL:             cfg.DatabaseURL,
			MaxConns:        cfg.DatabaseMaxConns,
			MinConns:        cfg.DatabaseMinConns,
			MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
			ConnectTimeout:  cfg.DatabaseConnectTimeout,
		})
	if err != nil {
		return err
	}

	tripRepo := repository.NewPgTripRepository(pool, cfg.DatabaseQueryTimeout)

	historyRepo := repository.NewPgTripStatusChangeRepository(pool, cfg.DatabaseQueryTimeout)

	txManager := infrastructure.NewTxManager(pool,
		cfg.DatabaseRollbackTimeout, cfg.DatabaseQueryTimeout)

	tripService := service.NewTripService(tripRepo, historyRepo, txManager,
		infrastructure.UUIDGenerator{})

	tripHandler := handler.NewTripHandler(tripService, cfg.HTTPMaxBodyBytes)
	healthHandler := handler.NewHealthHandler(pool, cfg.DatabaseQueryTimeout)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.NewRouter(tripHandler, healthHandler),
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	serverErrors := make(chan error, 1)

	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	var serveErr error

	select {
	case <-signalCtx.Done():

	case err := <-serverErrors:
		serveErr = fmt.Errorf("serve HTTP: %w", err)
	}

	stop()

	shutdownErr := shutdown(server, pool, cfg.ShutdownTimeout)

	return errors.Join(serveErr, shutdownErr)
}

func shutdown(server *http.Server, pool *pgxpool.Pool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	poolClosed := make(chan struct{})

	go func() {
		pool.Close()
		close(poolClosed)
	}()

	select {
	case <-poolClosed:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("close database pool: %w", ctx.Err())
	}
}
