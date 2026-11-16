package positions_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/worker/positions"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestScannerProcessesTicksSequentiallyAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockstaleTripRepository(ctrl)
		queue := NewMockqueue(ctrl)
		ticker := NewMockticker(ctrl)
		ticks := make(chan time.Time, 1)
		ticker.EXPECT().Ticks().Return((<-chan time.Time)(ticks))
		ticker.EXPECT().Stop()

		const staleAfter = time.Minute
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		firstScan := make(chan struct{})
		candidates := []trip.PositionCandidate{
			{TripID: "first"}, {TripID: "second"}, {TripID: "third"},
		}
		var queries atomic.Int32
		gomock.InOrder(
			repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter).DoAndReturn(func(scanCtx context.Context, _ time.Duration) ([]trip.PositionCandidate, error) {
				queries.Add(1)
				select {
				case <-firstScan:
					return candidates, nil
				case <-scanCtx.Done():
					return nil, scanCtx.Err()
				}
			}),
			queue.EXPECT().TryEnqueue(gomock.Any(), candidates[0]).Return(positions.Queued),
			queue.EXPECT().TryEnqueue(gomock.Any(), candidates[1]).Return(positions.Duplicate),
			queue.EXPECT().TryEnqueue(gomock.Any(), candidates[2]).Return(positions.Dropped),
			repo.EXPECT().ListStaleTrips(gomock.Any(), staleAfter).DoAndReturn(func(scanCtx context.Context, _ time.Duration) ([]trip.PositionCandidate, error) {
				queries.Add(1)
				<-scanCtx.Done()
				return nil, scanCtx.Err()
			}),
		)

		scanner := positions.NewScanner(repo, queue, staleAfter, slog.New(slog.DiscardHandler),
			tracenoop.NewTracerProvider().Tracer("test"), metricnoop.Int64Counter{}, metricnoop.Int64Counter{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			scanner.Run(ctx, ticker)
		}()

		ticks <- time.Now()
		synctest.Wait()
		ticks <- time.Now()
		synctest.Wait()
		if got := queries.Load(); got != 1 {
			t.Fatalf("while the first scan is blocked, got %d queries, want 1", got)
		}

		close(firstScan)
		synctest.Wait()
		if got := queries.Load(); got != 2 {
			t.Fatalf("after the first scan completes, got %d queries, want 2", got)
		}
		cancel()
		<-done
	})
}
