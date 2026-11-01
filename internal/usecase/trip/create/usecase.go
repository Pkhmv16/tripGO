package create

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/model"
)

type Usecase struct {
	tripRepo             tripRepository
	tripStatusChangeRepo tripStatusChangeRepository
	idempotencyRepo      idempotencyRepository
	txManager            txManager
	idGenerator          idGenerator
}

func New(repo tripRepository, history tripStatusChangeRepository, keys idempotencyRepository, tx txManager, ids idGenerator) *Usecase {
	return &Usecase{tripRepo: repo, tripStatusChangeRepo: history, idempotencyRepo: keys, txManager: tx, idGenerator: ids}
}

func (uc *Usecase) CreateTrip(ctx context.Context, p model.CreateTripParams) (*domaintrip.Trip, bool, error) {
	var requestHash []byte

	if p.IdempotencyKey != nil {
		var err error
		requestHash, err = createTripHash(p)
		if err != nil {
			return nil, false, err
		}
	}

	var result *domaintrip.Trip
	var created bool

	err := uc.txManager.Do(ctx, func(ctx context.Context) error {
		if p.IdempotencyKey != nil {
			record, err := uc.idempotencyRepo.FindForCreate(ctx, *p.IdempotencyKey)
			if err != nil {
				return err
			}

			if record != nil {
				if !bytes.Equal(record.RequestHash, requestHash) {
					return domaintrip.ErrIdempotencyConflict
				}

				trip, err := uc.tripRepo.Get(ctx, record.TripId)
				if err != nil {
					return err
				}

				result = trip
				return nil
			}
		}

		now := time.Now()
		tripId := domaintrip.TripId(uc.idGenerator.Generate())

		trip := domaintrip.NewTrip(
			tripId,
			p.UserId,
			p.DriverId,
			p.StartLatitude,
			p.StartLongitude,
			p.EndLatitude,
			p.EndLongitude,
			p.Price,
			now,
		)

		if err := uc.tripRepo.Create(ctx, trip); err != nil {
			return err
		}

		tripStatusChange := domaintrip.NewTripStatusChange(
			tripId,
			nil,
			"active",
			new("Trip initialized by usecase"),
			now)

		if err := uc.tripStatusChangeRepo.Save(ctx, tripStatusChange); err != nil {
			return err
		}

		if p.IdempotencyKey != nil {
			if err := uc.idempotencyRepo.Save(ctx, *p.IdempotencyKey, requestHash, tripId); err != nil {
				return err
			}
		}

		result = trip
		created = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}

	return result, created, nil
}

func createTripHash(p model.CreateTripParams) ([]byte, error) {
	for _, coordinate := range []*float64{
		&p.StartLatitude,
		&p.StartLongitude,
		&p.EndLatitude,
		&p.EndLongitude,
	} {
		if *coordinate == 0 {
			*coordinate = 0
		}
	}

	data, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode create trip parameters: %w", err)
	}

	sum := sha256.Sum256(data)
	return sum[:], nil
}
