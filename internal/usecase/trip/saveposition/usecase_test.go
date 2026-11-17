package saveposition_test

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/mock/gomock"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/saveposition"
)

type transactionContextKey struct{}

type fixture struct {
	ctx       context.Context
	txCtx     context.Context
	trips     *MocktripRepository
	positions *MockpositionRepository
	tx        *MocktxManager
	requests  *MockpositionRequestQueue
}

func (f *fixture) expectTransaction() {
	f.tx.EXPECT().Do(f.ctx, gomock.Any()).DoAndReturn(func(_ context.Context, run func(context.Context) error) error {
		return run(f.txCtx)
	})
}

func TestSavePosition(t *testing.T) {
	input := trip.Position{TripID: "trip-id", Latitude: 59.93, Longitude: 30.33, RecordedAt: time.Date(2025, 9, 1, 10, 5, 0, 123456789, time.UTC)}
	saved := input
	saved.ID = 42
	saved.RecordedAt = saved.RecordedAt.Truncate(time.Microsecond)
	active := &trip.Trip{Id: input.TripID, Status: trip.StatusActive}
	completed := &trip.Trip{Id: input.TripID, Status: trip.StatusCompleted}
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name    string
		setup   func(*fixture)
		want    trip.Position
		wantErr error
	}{
		{name: "success", want: saved, setup: func(f *fixture) {
			committed := f.tx.EXPECT().Do(f.ctx, gomock.Any()).
				DoAndReturn(func(_ context.Context, run func(context.Context) error) error { return run(f.txCtx) })
			gomock.InOrder(
				f.trips.EXPECT().GetForUpdate(f.txCtx, input.TripID).Return(active, nil),
				f.positions.EXPECT().Save(f.txCtx, input).Return(saved, nil),
				f.trips.EXPECT().UpdateLastPosition(f.txCtx, saved.TripID, saved.RecordedAt).Return(nil),
			)
			f.requests.EXPECT().Cancel(saved.TripID).After(committed)
		}},
		{name: "trip not found", wantErr: trip.ErrTripNotFound, setup: func(f *fixture) {
			f.expectTransaction()
			f.trips.EXPECT().GetForUpdate(f.txCtx, input.TripID).Return(nil, trip.ErrTripNotFound)
		}},
		{name: "trip completed", wantErr: trip.ErrTripAlreadyCompleted, setup: func(f *fixture) {
			f.expectTransaction()
			f.trips.EXPECT().GetForUpdate(f.txCtx, input.TripID).Return(completed, nil)
		}},
		{name: "read failed", wantErr: databaseErr, setup: func(f *fixture) {
			f.expectTransaction()
			f.trips.EXPECT().GetForUpdate(f.txCtx, input.TripID).Return(nil, databaseErr)
		}},
		{name: "insert failed", wantErr: databaseErr, setup: func(f *fixture) {
			f.expectTransaction()
			gomock.InOrder(
				f.trips.EXPECT().GetForUpdate(f.txCtx, input.TripID).Return(active, nil),
				f.positions.EXPECT().Save(f.txCtx, input).Return(trip.Position{}, databaseErr),
			)
		}},
		{name: "timestamp update failed", wantErr: databaseErr, setup: func(f *fixture) {
			f.expectTransaction()
			gomock.InOrder(
				f.trips.EXPECT().GetForUpdate(f.txCtx, input.TripID).Return(active, nil),
				f.positions.EXPECT().Save(f.txCtx, input).Return(saved, nil),
				f.trips.EXPECT().UpdateLastPosition(f.txCtx, saved.TripID, saved.RecordedAt).Return(databaseErr),
			)
		}},
		{name: "commit failed", wantErr: databaseErr, setup: func(f *fixture) {
			f.tx.EXPECT().Do(f.ctx, gomock.Any()).
				DoAndReturn(func(_ context.Context, run func(context.Context) error) error {
					if err := run(f.txCtx); err != nil {
						return err
					}
					return databaseErr
				})
			gomock.InOrder(
				f.trips.EXPECT().GetForUpdate(f.txCtx, input.TripID).Return(active, nil),
				f.positions.EXPECT().Save(f.txCtx, input).Return(saved, nil),
				f.trips.EXPECT().UpdateLastPosition(f.txCtx, saved.TripID, saved.RecordedAt).Return(nil),
			)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, uc := newUsecase(t)
			tt.setup(f)
			got, err := uc.SavePosition(f.ctx, input)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("position = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSavePositionValidation(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name  string
		input trip.Position
		field string
	}{
		{name: "latitude out of bounds", input: trip.Position{Latitude: 91, RecordedAt: now}, field: "coordinates.latitude"},
		{name: "longitude out of bounds", input: trip.Position{Longitude: -181, RecordedAt: now}, field: "coordinates.longitude"},
		{name: "recorded_at missing", field: "recorded_at"},
		{name: "recorded_at too far in future", input: trip.Position{RecordedAt: now.Add(2 * time.Minute)}, field: "recorded_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, uc := newUsecase(t)
			_, err := uc.SavePosition(f.ctx, tt.input)
			var validationErr *trip.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Field != tt.field {
				t.Errorf("error = %v, want validation error for %s", err, tt.field)
			}
		})
	}
}

func newUsecase(t *testing.T) (*fixture, *saveposition.Usecase) {
	t.Helper()
	ctrl := gomock.NewController(t)
	ctx := t.Context()
	f := &fixture{
		ctx: ctx, txCtx: context.WithValue(ctx, transactionContextKey{}, struct{}{}),
		trips: NewMocktripRepository(ctrl), positions: NewMockpositionRepository(ctrl), tx: NewMocktxManager(ctrl),
		requests: NewMockpositionRequestQueue(ctrl),
	}
	return f, saveposition.New(f.trips, f.positions, f.tx, f.requests, slog.New(slog.DiscardHandler), noop.Int64Counter{})
}
