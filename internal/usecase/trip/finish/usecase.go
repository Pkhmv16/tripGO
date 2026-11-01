package finish

import (
	"context"
	"time"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Usecase struct {
	tripRepo             tripRepository
	tripStatusChangeRepo tripStatusChangeRepository
	txManager            txManager
}

func New(repo tripRepository, history tripStatusChangeRepository, tx txManager) *Usecase {
	return &Usecase{tripRepo: repo, tripStatusChangeRepo: history, txManager: tx}
}

func (uc *Usecase) FinishTrip(ctx context.Context, tripId string) (*domaintrip.Trip, error) {
	var finalTrip *domaintrip.Trip
	domainTripId := domaintrip.TripId(tripId)

	err := uc.txManager.Do(ctx, func(ctx context.Context) error {
		trip, err := uc.tripRepo.GetForUpdate(ctx, domainTripId)
		if err != nil {
			return err
		}

		previousStatus := trip.Status
		now := time.Now()
		if err := trip.Finish(now); err != nil {
			return err
		}

		if err := uc.tripRepo.Update(ctx, trip); err != nil {
			return err
		}
		finalTrip = trip

		tripStatusChange := domaintrip.NewTripStatusChange(
			domainTripId,
			&previousStatus,
			trip.Status,
			new("Trip finished by usecase"),
			now,
		)

		return uc.tripStatusChangeRepo.Save(ctx, tripStatusChange)
	})
	if err != nil {
		return nil, err
	}

	return finalTrip, nil
}
