package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type PositionMetrics struct {
	StaleChecks      metric.Int64Counter
	PositionRequests metric.Int64Counter
	RequestDuration  metric.Float64Histogram
	RowsRead         metric.Int64Histogram
}

func NewPositionMetrics(meter metric.Meter) (PositionMetrics, error) {
	staleChecks, err := meter.Int64Counter("tripgo.stale_checks")
	if err != nil {
		return PositionMetrics{}, fmt.Errorf("create stale checks metric: %w", err)
	}
	requests, err := meter.Int64Counter("tripgo.position_requests")
	if err != nil {
		return PositionMetrics{}, fmt.Errorf("create position requests metric: %w", err)
	}
	duration, err := meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"))
	if err != nil {
		return PositionMetrics{}, fmt.Errorf("create position request duration metric: %w", err)
	}
	rowsRead, err := meter.Int64Histogram("tripgo.stale_checks.rows", metric.WithUnit("{row}"))
	if err != nil {
		return PositionMetrics{}, fmt.Errorf("create stale scan rows metric: %w", err)
	}
	return PositionMetrics{StaleChecks: staleChecks, PositionRequests: requests, RequestDuration: duration, RowsRead: rowsRead}, nil
}

func ObservePositionPolling(meter metric.Meter, queueSize func() int, activeWorkers func() int64) error {
	queue, err := meter.Int64ObservableGauge("tripgo.position_requests.queue.size")
	if err != nil {
		return fmt.Errorf("create position queue size metric: %w", err)
	}
	workers, err := meter.Int64ObservableGauge("tripgo.position_requests.workers.active")
	if err != nil {
		return fmt.Errorf("create active position workers metric: %w", err)
	}
	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(queue, int64(queueSize()))
		observer.ObserveInt64(workers, activeWorkers())
		return nil
	}, queue, workers)
	if err != nil {
		return fmt.Errorf("register position polling metrics: %w", err)
	}
	return nil
}
