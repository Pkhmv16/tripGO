package get

import (
	"context"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Usecase struct {
	tripRepo tripRepository
}

func New(repo tripRepository) *Usecase {
	return &Usecase{tripRepo: repo}
}

func (uc *Usecase) GetTrip(ctx context.Context, tripId string) (*domaintrip.Trip, error) {
	trip, err := uc.tripRepo.Get(ctx, domaintrip.TripId(tripId))
	if err != nil {
		return nil, err
	}
	return trip, nil
}
