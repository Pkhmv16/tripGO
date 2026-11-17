package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Pkhmv16/tripGO/internal/config"
	api "github.com/Pkhmv16/tripGO/internal/generated"
	transport "github.com/Pkhmv16/tripGO/internal/http"
	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
	"github.com/Pkhmv16/tripGO/internal/http/middleware"
	"github.com/Pkhmv16/tripGO/internal/http/probe/health"
	"github.com/Pkhmv16/tripGO/internal/http/probe/ready"
	"github.com/Pkhmv16/tripGO/internal/http/trip/create"
	"github.com/Pkhmv16/tripGO/internal/http/trip/finish"
	"github.com/Pkhmv16/tripGO/internal/http/trip/get"
	"github.com/Pkhmv16/tripGO/internal/http/trip/listpositions"
	"github.com/Pkhmv16/tripGO/internal/http/trip/saveposition"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
	"github.com/Pkhmv16/tripGO/internal/infra/telemetry"
	"github.com/Pkhmv16/tripGO/internal/infra/uuid"
	triprepository "github.com/Pkhmv16/tripGO/internal/repository/postgres/trip"
	tripidempotency "github.com/Pkhmv16/tripGO/internal/repository/postgres/tripidempotency"
	"github.com/Pkhmv16/tripGO/internal/repository/postgres/tripposition"
	"github.com/Pkhmv16/tripGO/internal/repository/postgres/tripstatushistory"
	createusecase "github.com/Pkhmv16/tripGO/internal/usecase/trip/create"
	finishusecase "github.com/Pkhmv16/tripGO/internal/usecase/trip/finish"
	getusecase "github.com/Pkhmv16/tripGO/internal/usecase/trip/get"
	listpositionsusecase "github.com/Pkhmv16/tripGO/internal/usecase/trip/listpositions"
	savepositionusecase "github.com/Pkhmv16/tripGO/internal/usecase/trip/saveposition"
)

type Container struct {
	server          *http.Server
	pool            *pgxpool.Pool
	logger          *slog.Logger
	tracing         *sdktrace.TracerProvider
	metrics         *sdkmetric.MeterProvider
	positions       *positionPolling
	shutdownTimeout time.Duration
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	logger := telemetry.NewLogger(os.Stdout, cfg.LogLevel)
	tracing, err := telemetry.NewTracerProvider(ctx, telemetry.TracingConfig{
		Endpoint:      cfg.OTelEndpoint,
		Protocol:      cfg.OTelProtocol,
		Sampler:       cfg.OTelSampler,
		Ratio:         cfg.OTelSamplerRatio,
		OTLPTimeout:   cfg.OTelOTLPTimeout,
		BatchDelay:    cfg.OTelTraceBatchDelay,
		ExportTimeout: cfg.OTelTraceExportTimeout,
	}, logger)
	if err != nil {
		return nil, err
	}
	metrics, err := telemetry.NewMeterProvider(ctx, telemetry.MetricsConfig{
		Endpoint:      cfg.OTelEndpoint,
		Protocol:      cfg.OTelProtocol,
		OTLPTimeout:   cfg.OTelOTLPTimeout,
		Interval:      cfg.OTelMetricInterval,
		ExportTimeout: cfg.OTelMetricTimeout,
	})
	if err != nil {
		return nil, errors.Join(err, tracing.Shutdown(ctx))
	}
	router := chi.NewRouter()
	metricsMiddleware, err := middleware.Metrics(metrics.Meter("github.com/Pkhmv16/tripGO/internal/http"), router)
	if err != nil {
		return nil, errors.Join(err, tracing.Shutdown(ctx), metrics.Shutdown(ctx))
	}
	router.Use(middleware.Logging(logger), metricsMiddleware)
	pool, err := postgres.NewPostgresPool(ctx,
		postgres.Config{
			URL:             cfg.DatabaseURL,
			MaxConns:        cfg.DatabaseMaxConns,
			MinConns:        cfg.DatabaseMinConns,
			MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
			ConnectTimeout:  cfg.DatabaseConnectTimeout,
		}, tracing, metrics)
	if err != nil {
		return nil, errors.Join(err, tracing.Shutdown(ctx), metrics.Shutdown(ctx))
	}

	tripRepo := triprepository.NewPgTripRepository(pool, cfg.DatabaseQueryTimeout)
	tripMetrics, err := telemetry.NewTripMetrics(ctx, metrics.Meter("github.com/Pkhmv16/tripGO/internal/usecase/trip"), tripRepo)
	if err != nil {
		pool.Close()
		return nil, errors.Join(err, tracing.Shutdown(ctx), metrics.Shutdown(ctx))
	}
	polling, err := newPositionPolling(cfg, tripRepo, logger, tracing, metrics)
	if err != nil {
		shutdownErr := errors.Join(metrics.Shutdown(ctx), tracing.Shutdown(ctx))
		pool.Close()
		return nil, errors.Join(err, shutdownErr)
	}

	historyRepo := tripstatushistory.NewPgTripStatusChangeRepository(pool, cfg.DatabaseQueryTimeout)

	txManager := transaction.NewTxManager(pool,
		cfg.DatabaseRollbackTimeout, cfg.DatabaseQueryTimeout)

	idempotencyRepo := tripidempotency.NewPgIdempotencyRepository(pool, cfg.DatabaseQueryTimeout, cfg.IdempotencyTTL)

	tracer := tracing.Tracer("github.com/Pkhmv16/tripGO/internal/usecase/trip")
	createUsecase := createusecase.New(tripRepo, historyRepo, idempotencyRepo, txManager, uuid.Generator{}, logger, tracer, tripMetrics.Created)
	getUsecase := getusecase.New(tripRepo, logger, tracer)
	finishUsecase := finishusecase.New(tripRepo, historyRepo, txManager, logger, tracer, tripMetrics.Completed)
	positionRepo := tripposition.New(pool, cfg.DatabaseQueryTimeout)
	savePositionUsecase := savepositionusecase.New(tripRepo, positionRepo, txManager, polling.queue, logger, tripMetrics.PositionsSaved)
	listPositionsUsecase := listpositionsusecase.New(tripRepo, positionRepo, logger)

	createHandler := create.New(createUsecase, cfg.HTTPMaxBodyBytes, logger)
	getHandler := get.New(getUsecase, logger)
	finishHandler := finish.New(finishUsecase, logger)
	healthHandler := health.New()
	readyHandler := ready.New(pool, cfg.DatabaseQueryTimeout, logger)

	api.HandlerWithOptions(&transport.Handler{
		CreateHandler:        createHandler,
		GetHandler:           getHandler,
		FinishHandler:        finishHandler,
		HealthHandler:        healthHandler,
		ReadyHandler:         readyHandler,
		SavePositionHandler:  saveposition.New(savePositionUsecase, cfg.HTTPMaxBodyBytes),
		ListPositionsHandler: listpositions.New(listPositionsUsecase, logger),
	}, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			domainerror.WriteInvalidRequest(w, r, "Invalid request parameter")
		},
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           middleware.Recover(logger)(middleware.Tracing(tracing, router)),
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	return &Container{
		server: server, pool: pool, logger: logger, tracing: tracing, metrics: metrics,
		positions: polling, shutdownTimeout: cfg.ShutdownTimeout,
	}, nil
}

func (c *Container) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", c.server.Addr)
	if err != nil {
		return errors.Join(fmt.Errorf("listen HTTP: %w", err), c.shutdown(ctx, nil))
	}
	stopPolling := c.positions.start(context.WithoutCancel(ctx))
	c.logger.Info("service started", "address", listener.Addr().String())
	serverDone := make(chan error, 1)
	go func() {
		err := c.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		} else if err != nil {
			err = fmt.Errorf("serve HTTP: %w", err)
		}
		serverDone <- err
	}()

	select {
	case err := <-serverDone:
		return errors.Join(err, c.shutdown(ctx, stopPolling))
	case <-ctx.Done():
		shutdownErr := c.shutdown(ctx, stopPolling)
		return errors.Join(<-serverDone, shutdownErr)
	}
}

func (c *Container) shutdown(parent context.Context, stopPolling func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), c.shutdownTimeout)
	defer cancel()

	c.logger.InfoContext(ctx, "service stopping")
	var shutdownErr error
	if err := c.server.Shutdown(ctx); err != nil {
		shutdownErr = fmt.Errorf("shutdown HTTP: %w", err)
		if closeErr := c.server.Close(); closeErr != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close HTTP: %w", closeErr))
		}
	}
	if stopPolling != nil {
		shutdownErr = errors.Join(shutdownErr, stopPolling(ctx))
	}
	c.positions.push.CloseIdleConnections()
	shutdownErr = errors.Join(shutdownErr, c.metrics.Shutdown(ctx))
	poolClosed := make(chan struct{})
	go func() {
		c.pool.Close()
		close(poolClosed)
	}()
	select {
	case <-poolClosed:
		shutdownErr = errors.Join(shutdownErr, c.tracing.Shutdown(ctx))
		if shutdownErr == nil {
			c.logger.InfoContext(ctx, "service stopped")
		}
		return shutdownErr
	case <-ctx.Done():
		return errors.Join(shutdownErr, fmt.Errorf("close database pool: %w", ctx.Err()), c.tracing.Shutdown(ctx))
	}
}
