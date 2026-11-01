package trip

import (
	"time"
)

type TripId string
type UserId string

type DriverId string

type Trip struct {
	Id             TripId
	UserId         UserId
	DriverId       DriverId
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	Status         TripStatus
	StartedAt      time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewTrip(
	id TripId,
	userId UserId,
	driverId DriverId,
	startLatitude float64,
	startLongitude float64,
	endLatitude float64,
	endLongitude float64,
	price int64,
	now time.Time,
) *Trip {
	return &Trip{
		Id:             id,
		UserId:         userId,
		DriverId:       driverId,
		StartLatitude:  startLatitude,
		StartLongitude: startLongitude,
		EndLongitude:   endLongitude,
		EndLatitude:    endLatitude,
		Price:          price,
		Status:         StatusActive,
		StartedAt:      now,
		FinishedAt:     nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func (t *Trip) Finish(now time.Time) error {
	if t.Status != StatusActive {
		return ErrTripAlreadyCompleted
	}

	t.Status = StatusCompleted
	t.FinishedAt = &now
	t.UpdatedAt = now
	return nil
}
