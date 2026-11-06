package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type activeTripCounter interface {
	CountActive(context.Context) (int64, error)
}

type TripMetrics struct {
	Created   metric.Int64Counter
	Completed metric.Int64Counter
}

func NewTripMetrics(ctx context.Context, meter metric.Meter, trips activeTripCounter) (*TripMetrics, error) {
	created, err := meter.Int64Counter("tripgo.trips.created", metric.WithUnit("{trip}"), metric.WithDescription("Number of trips created"))
	if err != nil {
		return nil, fmt.Errorf("create trips created metric: %w", err)
	}
	completed, err := meter.Int64Counter("tripgo.trips.completed", metric.WithUnit("{trip}"), metric.WithDescription("Number of trips completed"))
	if err != nil {
		return nil, fmt.Errorf("create trips completed metric: %w", err)
	}
	active, err := meter.Int64ObservableGauge("tripgo.trips.active", metric.WithUnit("{trip}"), metric.WithDescription("Number of active trips"))
	if err != nil {
		return nil, fmt.Errorf("create active trips metric: %w", err)
	}
	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		count, err := trips.CountActive(ctx)
		if err != nil {
			return fmt.Errorf("observe active trips: %w", err)
		}
		observer.ObserveInt64(active, count)
		return nil
	}, active)
	if err != nil {
		return nil, fmt.Errorf("register active trips callback: %w", err)
	}
	created.Add(ctx, 0)
	completed.Add(ctx, 0)
	return &TripMetrics{Created: created, Completed: completed}, nil
}
