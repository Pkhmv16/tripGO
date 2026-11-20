package saveposition_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/saveposition"
	"github.com/Pkhmv16/tripGO/internal/worker/positions"
)

type pushFunc func(context.Context, trip.PositionCandidate) error

func (f pushFunc) RequestPosition(ctx context.Context, candidate trip.PositionCandidate) error {
	return f(ctx, candidate)
}

func TestSavePositionCancelsPendingPushOnlyAfterCommit(t *testing.T) {
	const otherTripID trip.TripId = "other"
	input := trip.Position{TripID: "updated", RecordedAt: time.Now().UTC()}
	active := &trip.Trip{Id: input.TripID, Status: trip.StatusActive}
	commitErr := errors.New("commit failed")
	tests := []struct {
		name     string
		setup    func(*fixture)
		wantErr  error
		wantSent []trip.TripId
	}{
		{
			name: "committed position cancels its pending push",
			setup: func(f *fixture) {
				f.expectTransaction()
			},
			wantSent: []trip.TripId{otherTripID},
		},
		{
			name: "failed commit preserves pending push",
			setup: func(f *fixture) {
				f.tx.EXPECT().Do(f.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, run func(context.Context) error) error {
						if err := run(f.txCtx); err != nil {
							return err
						}
						return commitErr
					})
			},
			wantErr:  commitErr,
			wantSent: []trip.TripId{input.TripID, otherTripID},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := newUsecase(t)
			tt.setup(f)
			gomock.InOrder(
				f.trips.EXPECT().GetForUpdate(f.txCtx, input.TripID).Return(active, nil),
				f.positions.EXPECT().Save(f.txCtx, input).Return(input, nil),
				f.trips.EXPECT().UpdateLastPosition(f.txCtx, input.TripID, input.RecordedAt).Return(nil),
			)
			queue := positions.NewQueue(2)
			defer queue.Close()
			for _, id := range []trip.TripId{input.TripID, otherTripID} {
				if got := queue.TryEnqueue(f.ctx, trip.PositionCandidate{TripID: id}); got != positions.Queued {
					t.Fatalf("enqueue %s = %q, want queued", id, got)
				}
			}
			logger := slog.New(slog.DiscardHandler)
			counter := metricnoop.Int64Counter{}
			uc := saveposition.New(f.trips, f.positions, f.tx, queue, logger, counter)
			if _, err := uc.SavePosition(f.ctx, input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("SavePosition error = %v, want %v", err, tt.wantErr)
			}

			var sent []trip.TripId
			push := pushFunc(func(_ context.Context, candidate trip.PositionCandidate) error {
				sent = append(sent, candidate.TripID)
				return nil
			})
			queue.Close()
			pool := positions.NewPool(queue, push, 1, time.Second, logger,
				tracenoop.NewTracerProvider().Tracer("test"), counter, metricnoop.Float64Histogram{})
			pool.Run(f.ctx)
			if !slices.Equal(sent, tt.wantSent) {
				t.Fatalf("sent trips = %v, want %v", sent, tt.wantSent)
			}
		})
	}
}
