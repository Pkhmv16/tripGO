package saveposition

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Usecase struct {
	trips     tripRepository
	positions positionRepository
	tx        txManager
	requests  positionRequestQueue
	logger    logger
	saved     metric.Int64Counter
}

func New(trips tripRepository, positions positionRepository, tx txManager, requests positionRequestQueue, log logger, saved metric.Int64Counter) *Usecase {
	return &Usecase{trips: trips, positions: positions, tx: tx, requests: requests, logger: log, saved: saved}
}

func (uc *Usecase) SavePosition(ctx context.Context, position trip.Position) (trip.Position, error) {
	if err := validate(position, time.Now()); err != nil {
		return trip.Position{}, err
	}

	var saved trip.Position
	err := uc.tx.Do(ctx, func(ctx context.Context) error {
		current, err := uc.trips.GetForUpdate(ctx, position.TripID)
		if err != nil {
			return err
		}
		if current.Status != trip.StatusActive {
			return trip.ErrTripAlreadyCompleted
		}
		saved, err = uc.positions.Save(ctx, position)
		if err != nil {
			return err
		}
		return uc.trips.UpdateLastPosition(ctx, saved.TripID, saved.RecordedAt)
	})
	if err != nil {
		if !errors.Is(err, trip.ErrTripNotFound) && !errors.Is(err, trip.ErrTripAlreadyCompleted) {
			uc.logger.ErrorContext(ctx, "trip position save failed", "trip_id", position.TripID, "error", err)
		}
		return trip.Position{}, err
	}
	uc.requests.Cancel(saved.TripID)
	uc.saved.Add(ctx, 1)
	return saved, nil
}

func validate(position trip.Position, now time.Time) error {
	if !(position.Latitude >= -90 && position.Latitude <= 90) {
		return &trip.ValidationError{Field: "coordinates.latitude", Reason: "must be between -90 and 90"}
	}
	if !(position.Longitude >= -180 && position.Longitude <= 180) {
		return &trip.ValidationError{Field: "coordinates.longitude", Reason: "must be between -180 and 180"}
	}
	if position.RecordedAt.IsZero() || position.RecordedAt.After(now.Add(time.Minute)) {
		return &trip.ValidationError{Field: "recorded_at", Reason: "must be present and at most one minute ahead"}
	}
	return nil
}
