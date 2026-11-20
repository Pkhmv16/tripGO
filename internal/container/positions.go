package container

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/Pkhmv16/tripGO/internal/config"
	"github.com/Pkhmv16/tripGO/internal/gateway/push"
	"github.com/Pkhmv16/tripGO/internal/infra/clock"
	"github.com/Pkhmv16/tripGO/internal/infra/telemetry"
	triprepository "github.com/Pkhmv16/tripGO/internal/repository/postgres/trip"
	"github.com/Pkhmv16/tripGO/internal/worker/positions"
)

type positionPolling struct {
	queue    *positions.Queue
	scanner  *positions.Scanner
	workers  *positions.Pool
	push     *push.Gateway
	interval time.Duration
}

func newPositionPolling(cfg config.Config, trips *triprepository.PgTripRepository, logger *slog.Logger, tracing trace.TracerProvider, metrics metric.MeterProvider) (*positionPolling, error) {
	const scope = "github.com/Pkhmv16/tripGO/internal/worker/positions"
	meter := metrics.Meter(scope)
	instruments, err := telemetry.NewPositionMetrics(meter)
	if err != nil {
		return nil, err
	}
	gateway, err := push.New(cfg.PushHTTPURL, cfg.PushRequestTimeout, tracing)
	if err != nil {
		return nil, fmt.Errorf("create push gateway: %w", err)
	}
	queue := positions.NewQueue(cfg.PositionQueueSize)
	tracer := tracing.Tracer(scope)
	scanner := positions.NewScanner(trips, queue, cfg.PositionStaleAfter, cfg.PositionBatchSize, logger, tracer, instruments.StaleChecks, instruments.PositionRequests, instruments.RowsRead)
	workers := positions.NewPool(queue, gateway, cfg.PositionWorkers, cfg.PushRequestTimeout, logger, tracer, instruments.PositionRequests, instruments.RequestDuration)
	if err := telemetry.ObservePositionPolling(meter, queue.Len, workers.Active); err != nil {
		return nil, err
	}
	return &positionPolling{queue: queue, scanner: scanner, workers: workers, push: gateway, interval: cfg.PositionCheckInterval}, nil
}

func (p *positionPolling) start(ctx context.Context) func(context.Context) error {
	scanCtx, stopScanner := context.WithCancel(ctx)
	workersCtx, stopWorkers := context.WithCancel(ctx)
	scannerDone := make(chan struct{})
	workersDone := make(chan struct{})
	ticker := clock.NewTicker(p.interval)
	go func() {
		defer close(scannerDone)
		p.scanner.Run(scanCtx, ticker)
	}()
	go func() {
		defer close(workersDone)
		p.workers.Run(workersCtx)
	}()

	return func(shutdownCtx context.Context) error {
		stopAtDeadline := context.AfterFunc(shutdownCtx, stopWorkers)
		defer stopAtDeadline()
		defer stopWorkers()

		stopScanner()
		<-scannerDone
		p.queue.Close()
		<-workersDone
		if err := shutdownCtx.Err(); err != nil {
			return fmt.Errorf("drain position polling: %w", err)
		}
		return nil
	}
}
