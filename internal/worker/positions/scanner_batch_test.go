package positions_test

import (
	"context"
	"fmt"
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

func TestScannerReads250TripsInThreeBatches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockstaleTripRepository(ctrl)
		queue := positions.NewQueue(250)
		defer queue.Close()
		candidates := make([]trip.PositionCandidate, 250)
		lastActivity := time.Now().Add(-time.Hour)
		for i := range candidates {
			candidates[i] = trip.PositionCandidate{
				TripID: trip.TripId(fmt.Sprintf("trip-%03d", i)), LastActivity: lastActivity,
			}
		}
		gomock.InOrder(
			repo.EXPECT().ListStaleTrips(gomock.Any(), time.Minute, 100, nil).
				Return(candidates[:100], nil),
			repo.EXPECT().ListStaleTrips(gomock.Any(), time.Minute, 100,
				&trip.PositionCursor{LastActivity: lastActivity, TripID: candidates[99].TripID}).
				Return(candidates[100:200], nil),
			repo.EXPECT().ListStaleTrips(gomock.Any(), time.Minute, 100,
				&trip.PositionCursor{LastActivity: lastActivity, TripID: candidates[199].TripID}).
				Return(candidates[200:], nil),
		)
		ticker := NewMockticker(ctrl)
		ticks := make(chan time.Time)
		ticker.EXPECT().Ticks().Return((<-chan time.Time)(ticks))
		ticker.EXPECT().Stop()
		scanner := positions.NewScanner(repo, queue, time.Minute, 100, slog.New(slog.DiscardHandler),
			tracenoop.NewTracerProvider().Tracer("test"), metricnoop.Int64Counter{}, metricnoop.Int64Counter{}, metricnoop.Int64Histogram{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() {
			defer close(done)
			scanner.Run(ctx, ticker)
		}()
		ticks <- time.Now()
		synctest.Wait()
		cancel()
		<-done
		if got := queue.Len(); got != len(candidates) {
			t.Fatalf("queued trips = %d, want %d", got, len(candidates))
		}

		push := NewMockpushGateway(ctrl)
		for _, candidate := range candidates {
			push.EXPECT().RequestPosition(gomock.Any(), candidate).Return(nil)
		}
		queue.Close()
		newTestPool(queue, push, 1).Run(t.Context())
	})
}

func TestScannerStopsBatchOnFullQueueAndRestartsNextTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockstaleTripRepository(ctrl)
		queue := positions.NewQueue(1)
		defer queue.Close()
		candidates := []trip.PositionCandidate{
			{TripID: "accepted"}, {TripID: "dropped"},
		}
		repo.EXPECT().ListStaleTrips(gomock.Any(), time.Minute, 2, nil).
			Return(candidates, nil).Times(2)
		ticker := NewMockticker(ctrl)
		ticks := make(chan time.Time)
		ticker.EXPECT().Ticks().Return((<-chan time.Time)(ticks))
		ticker.EXPECT().Stop()
		scanner := positions.NewScanner(repo, queue, time.Minute, 2, slog.New(slog.DiscardHandler),
			tracenoop.NewTracerProvider().Tracer("test"), metricnoop.Int64Counter{}, metricnoop.Int64Counter{}, metricnoop.Int64Histogram{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() {
			defer close(done)
			scanner.Run(ctx, ticker)
		}()
		for range 2 {
			ticks <- time.Now()
			synctest.Wait()
		}
		cancel()
		<-done

		push := NewMockpushGateway(ctrl)
		push.EXPECT().RequestPosition(gomock.Any(), candidates[0]).Return(nil)
		queue.Close()
		newTestPool(queue, push, 1).Run(t.Context())
	})
}
