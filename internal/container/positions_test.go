package container

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/worker/positions"
)

type pushFunc func(context.Context, trip.PositionCandidate) error

func (f pushFunc) RequestPosition(ctx context.Context, candidate trip.PositionCandidate) error {
	return f(ctx, candidate)
}

func TestPositionPollingDrainsOnShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		requests := make(chan trip.TripId, 2)
		release := make(chan struct{})
		polling := testPositionPolling(t, func(ctx context.Context, candidate trip.PositionCandidate) error {
			select {
			case <-release:
				requests <- candidate.TripID
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		stop := polling.start(t.Context())
		synctest.Wait()

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- stop(ctx) }()
		synctest.Wait()
		close(release)

		if err := <-done; err != nil {
			t.Fatalf("stop: %v", err)
		}
		close(requests)
		first, second := <-requests, <-requests
		if first != "first" || second != "second" {
			t.Fatalf("sent requests = %q, %q, want first and second", first, second)
		}
	})
}

func TestPositionPollingCancelsInFlightRequestAtShutdownDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		requests := make(chan error, 2)
		polling := testPositionPolling(t, func(ctx context.Context, _ trip.PositionCandidate) error {
			<-ctx.Done()
			requests <- ctx.Err()
			return ctx.Err()
		})
		stop := polling.start(t.Context())
		synctest.Wait()

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stop = %v, want shutdown deadline exceeded", err)
		}
		close(requests)
		if err := <-requests; !errors.Is(err, context.Canceled) {
			t.Fatalf("request error = %v, want cancelled", err)
		}
		if _, ok := <-requests; ok {
			t.Fatal("worker started another request after the shutdown deadline")
		}
	})
}

func testPositionPolling(t *testing.T, push pushFunc) *positionPolling {
	t.Helper()
	queue := positions.NewQueue(2)
	for _, id := range []trip.TripId{"first", "second"} {
		if got := queue.TryEnqueue(t.Context(), trip.PositionCandidate{TripID: id}); got != positions.Queued {
			t.Fatalf("enqueue %s = %q, want queued", id, got)
		}
	}
	logger := slog.New(slog.DiscardHandler)
	tracer := tracenoop.NewTracerProvider().Tracer("test")
	counter := metricnoop.Int64Counter{}
	return &positionPolling{
		queue:    queue,
		scanner:  positions.NewScanner(nil, queue, time.Minute, logger, tracer, counter, counter),
		workers:  positions.NewPool(queue, push, 1, time.Minute, logger, tracer, counter, metricnoop.Float64Histogram{}),
		interval: time.Hour,
	}
}
