package listpositions_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/listpositions"
)

type fixture struct {
	ctx       context.Context
	trips     *MocktripRepository
	positions *MockpositionRepository
}

func TestListPositions(t *testing.T) {
	const id trip.TripId = "trip-id"
	completed := &trip.Trip{Id: id, Status: trip.StatusCompleted}
	points := []trip.Position{{ID: 1, TripID: id}, {ID: 2, TripID: id}}
	databaseErr := errors.New("database unavailable")
	tests := []struct {
		name    string
		setup   func(*fixture)
		want    []trip.Position
		wantErr error
	}{
		{name: "completed trip", want: points, setup: func(f *fixture) {
			gomock.InOrder(
				f.trips.EXPECT().Get(f.ctx, id).Return(completed, nil),
				f.positions.EXPECT().ListByTrip(f.ctx, id).Return(points, nil),
			)
		}},
		{name: "trip not found", wantErr: trip.ErrTripNotFound, setup: func(f *fixture) {
			f.trips.EXPECT().Get(f.ctx, id).Return(nil, trip.ErrTripNotFound)
		}},
		{name: "trip lookup failed", wantErr: databaseErr, setup: func(f *fixture) {
			f.trips.EXPECT().Get(f.ctx, id).Return(nil, databaseErr)
		}},
		{name: "positions lookup failed", wantErr: databaseErr, setup: func(f *fixture) {
			gomock.InOrder(
				f.trips.EXPECT().Get(f.ctx, id).Return(completed, nil),
				f.positions.EXPECT().ListByTrip(f.ctx, id).Return(nil, databaseErr),
			)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			f := &fixture{ctx: t.Context(), trips: NewMocktripRepository(ctrl), positions: NewMockpositionRepository(ctrl)}
			tt.setup(f)
			uc := listpositions.New(f.trips, f.positions, slog.New(slog.DiscardHandler))
			got, err := uc.ListPositions(f.ctx, id)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("positions = %+v, want %+v", got, tt.want)
			}
		})
	}
}
