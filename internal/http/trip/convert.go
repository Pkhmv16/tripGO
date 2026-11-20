package trip

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	api "github.com/Pkhmv16/tripGO/internal/generated"
)

func ToApiTrip(trip *trip.Trip) (*api.Trip, error) {
	id, err := uuid.Parse(string(trip.Id))
	if err != nil {
		return nil, fmt.Errorf("convert trip id: %w", err)
	}

	userID, err := uuid.Parse(string(trip.UserId))
	if err != nil {
		return nil, fmt.Errorf("convert user id: %w", err)
	}

	driverID, err := uuid.Parse(string(trip.DriverId))
	if err != nil {
		return nil, fmt.Errorf("convert driver id: %w", err)
	}

	return &api.Trip{
		Id:       id,
		UserId:   userID,
		DriverId: driverID,
		StartPoint: api.Coordinates{
			Latitude:  trip.StartLatitude,
			Longitude: trip.StartLongitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  trip.EndLatitude,
			Longitude: trip.EndLongitude,
		},
		Price:          trip.Price,
		Status:         api.TripStatus(trip.Status),
		StartedAt:      trip.StartedAt,
		FinishedAt:     trip.FinishedAt,
		LastPositionAt: trip.LastPositionAt,
	}, nil
}

func ToApiTripPosition(position trip.Position) (api.TripPosition, error) {
	id, err := uuid.Parse(string(position.TripID))
	if err != nil {
		return api.TripPosition{}, fmt.Errorf("convert position trip id: %w", err)
	}
	return api.TripPosition{
		Id:          position.ID,
		TripId:      id,
		Coordinates: api.Coordinates{Latitude: position.Latitude, Longitude: position.Longitude},
		RecordedAt:  position.RecordedAt,
	}, nil
}
