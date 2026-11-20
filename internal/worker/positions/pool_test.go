package positions_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/worker/positions"
)

func TestPoolLimitsConcurrentRequestsAndDrainsClosedQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const workerCount = 2
		candidates := []trip.PositionCandidate{
			{TripID: "first"}, {TripID: "second"}, {TripID: "third"},
		}
		queue := enqueueCandidates(t, candidates...)
		queue.Close()

		push := NewMockpushGateway(gomock.NewController(t))
		started := make(chan struct{}, len(candidates))
		release := make(chan struct{})
		for _, candidate := range candidates {
			push.EXPECT().RequestPosition(gomock.Any(), candidate).
				DoAndReturn(func(_ context.Context, _ trip.PositionCandidate) error {
					started <- struct{}{}
					<-release
					return nil
				})
		}

		pool := newTestPool(queue, push, workerCount)
		done := make(chan struct{})
		go func() {
			pool.Run(t.Context())
			close(done)
		}()
		synctest.Wait()
		concurrentRequests := len(started)
		close(release)
		<-done

		if concurrentRequests != workerCount {
			t.Fatalf("concurrent requests = %d, want %d", concurrentRequests, workerCount)
		}
		if got := len(started); got != len(candidates) {
			t.Fatalf("requests after draining = %d, want %d", got, len(candidates))
		}
	})
}

func TestPoolContinuesAfterPushErrorAndTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failed := trip.PositionCandidate{TripID: "failed"}
		slow := trip.PositionCandidate{TripID: "slow"}
		skipped := trip.PositionCandidate{TripID: "cancelled"}
		last := trip.PositionCandidate{TripID: "last"}
		queue := enqueueCandidates(t, failed, slow, skipped, last)
		queue.Cancel(skipped.TripID)

		push := NewMockpushGateway(gomock.NewController(t))
		var timeoutErr error
		gomock.InOrder(
			push.EXPECT().RequestPosition(gomock.Any(), failed).Return(errors.New("push unavailable")),
			push.EXPECT().RequestPosition(gomock.Any(), slow).
				DoAndReturn(func(ctx context.Context, _ trip.PositionCandidate) error {
					<-ctx.Done()
					timeoutErr = ctx.Err()
					return timeoutErr
				}),
			push.EXPECT().RequestPosition(gomock.Any(), last).Return(nil),
		)

		queue.Close()
		newTestPool(queue, push, 1).Run(t.Context())
		if !errors.Is(timeoutErr, context.DeadlineExceeded) {
			t.Fatalf("slow request error = %v, want deadline exceeded", timeoutErr)
		}
	})
}

func TestPoolStopsWhenContextIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inFlight := trip.PositionCandidate{TripID: "in-flight"}
		pending := trip.PositionCandidate{TripID: "pending"}
		queue := enqueueCandidates(t, inFlight, pending)
		defer queue.Close()

		push := NewMockpushGateway(gomock.NewController(t))
		started := make(chan struct{})
		var requestErr error
		push.EXPECT().RequestPosition(gomock.Any(), inFlight).
			DoAndReturn(func(ctx context.Context, _ trip.PositionCandidate) error {
				close(started)
				<-ctx.Done()
				requestErr = ctx.Err()
				return requestErr
			})

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		pool := newTestPool(queue, push, 1)
		done := make(chan struct{})
		go func() {
			pool.Run(ctx)
			close(done)
		}()
		<-started
		cancel()
		<-done

		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("in-flight request error = %v, want cancelled", requestErr)
		}
		if got := queue.TryEnqueue(t.Context(), inFlight); got != positions.Queued {
			t.Fatalf("enqueue after processing = %q, want queued", got)
		}
	})
}

func enqueueCandidates(t *testing.T, candidates ...trip.PositionCandidate) *positions.Queue {
	t.Helper()
	queue := positions.NewQueue(len(candidates))
	for _, candidate := range candidates {
		if got := queue.TryEnqueue(t.Context(), candidate); got != positions.Queued {
			t.Fatalf("enqueue %s = %q, want queued", candidate.TripID, got)
		}
	}
	return queue
}

func newTestPool(queue *positions.Queue, push *MockpushGateway, workers int) *positions.Pool {
	return positions.NewPool(queue, push, workers, time.Second, slog.New(slog.DiscardHandler),
		tracenoop.NewTracerProvider().Tracer("test"), metricnoop.Int64Counter{}, metricnoop.Float64Histogram{})
}
