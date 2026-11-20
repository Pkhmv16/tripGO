package listpositions

import (
	"context"
	"errors"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Usecase struct {
	trips     tripRepository
	positions positionRepository
	logger    logger
}

func New(trips tripRepository, positions positionRepository, log logger) *Usecase {
	return &Usecase{trips: trips, positions: positions, logger: log}
}

func (uc *Usecase) ListPositions(ctx context.Context, id trip.TripId) ([]trip.Position, error) {
	if _, err := uc.trips.Get(ctx, id); err != nil {
		if !errors.Is(err, trip.ErrTripNotFound) {
			uc.logger.ErrorContext(ctx, "trip lookup for route failed", "trip_id", id, "error", err)
		}
		return nil, err
	}
	positions, err := uc.positions.ListByTrip(ctx, id)
	if err != nil {
		uc.logger.ErrorContext(ctx, "trip positions retrieval failed", "trip_id", id, "error", err)
		return nil, err
	}
	return positions, nil
}
