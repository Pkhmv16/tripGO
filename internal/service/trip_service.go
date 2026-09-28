package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Pkhmv16/tripGO/internal/domain"
)

type TripRepo interface {
	Get(ctx context.Context, tripId domain.TripId) (*domain.Trip, error)
	Create(ctx context.Context, trip *domain.Trip) error
	Finish(ctx context.Context, tripId domain.TripId, finishedAt time.Time) (bool, error)
}

type TripStatusChangeRepo interface {
	Save(ctx context.Context, tripStatusChange *domain.TripStatusChange) error
}

type IdempotencyRepo interface {
	FindForCreate(ctx context.Context, key string) (*domain.IdempotencyRecord, error)
	Save(ctx context.Context, key string, requestHash []byte, tripId domain.TripId) error
}

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type IdGenerator interface {
	Generate() string
}

type TripService struct {
	tripRepo             TripRepo
	tripStatusChangeRepo TripStatusChangeRepo
	idempotencyRepo      IdempotencyRepo
	txManager            TxManager
	idGenerator          IdGenerator
}

func NewTripService(
	tripRepo TripRepo,
	tripStatusChangeRepo TripStatusChangeRepo,
	idempotencyRepo IdempotencyRepo,
	txManager TxManager,
	idGenerator IdGenerator) *TripService {
	return &TripService{
		tripRepo:             tripRepo,
		tripStatusChangeRepo: tripStatusChangeRepo,
		idempotencyRepo:      idempotencyRepo,
		txManager:            txManager,
		idGenerator:          idGenerator,
	}
}

type CreateTripParams struct {
	UserId         domain.UserId
	DriverId       domain.DriverId
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	IdempotencyKey *string `json:"-"`
}

func (ts *TripService) CreateTrip(ctx context.Context, p CreateTripParams) (*domain.Trip, bool, error) {
	var requestHash []byte

	if p.IdempotencyKey != nil {
		var err error
		requestHash, err = createTripHash(p)
		if err != nil {
			return nil, false, err
		}
	}

	var result *domain.Trip
	var created bool

	err := ts.txManager.Do(ctx, func(ctx context.Context) error {
		if p.IdempotencyKey != nil {
			record, err := ts.idempotencyRepo.FindForCreate(ctx, *p.IdempotencyKey)
			if err != nil {
				return err
			}

			if record != nil {
				if !bytes.Equal(record.RequestHash, requestHash) {
					return domain.ErrIdempotencyConflict
				}

				trip, err := ts.tripRepo.Get(ctx, record.TripId)
				if err != nil {
					return err
				}

				result = trip
				return nil
			}
		}

		now := time.Now()
		tripId := domain.TripId(ts.idGenerator.Generate())

		trip := domain.NewTrip(
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

		if err := ts.tripRepo.Create(ctx, trip); err != nil {
			return err
		}

		tripStatusChange := domain.NewTripStatusChange(
			tripId,
			nil,
			"active",
			new("Trip initialized by service"),
			now)

		if err := ts.tripStatusChangeRepo.Save(ctx, tripStatusChange); err != nil {
			return err
		}

		if p.IdempotencyKey != nil {
			if err := ts.idempotencyRepo.Save(ctx, *p.IdempotencyKey, requestHash, tripId); err != nil {
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

func (ts *TripService) GetTrip(ctx context.Context, tripId string) (*domain.Trip, error) {
	trip, err := ts.tripRepo.Get(ctx, domain.TripId(tripId))
	if err != nil {
		return nil, err
	}
	return trip, nil
}

func (ts *TripService) FinishTrip(ctx context.Context, tripId string) (*domain.Trip, error) {
	var finalTrip *domain.Trip
	domainTripId := domain.TripId(tripId)
	now := time.Now()

	err := ts.txManager.Do(ctx, func(ctx context.Context) error {
		updated, err := ts.tripRepo.Finish(ctx, domainTripId, now)
		if err != nil {
			return err
		}

		if !updated {
			_, err := ts.tripRepo.Get(ctx, domainTripId)
			if err != nil {
				return err
			}

			return domain.ErrTripAlreadyCompleted
		}

		trip, err := ts.tripRepo.Get(ctx, domainTripId)
		if err != nil {
			return err
		}
		finalTrip = trip

		tripStatusChange := domain.NewTripStatusChange(
			domainTripId,
			new(domain.TripStatus("active")),
			"completed",
			new("Trip finished by service"),
			now)

		return ts.tripStatusChangeRepo.Save(ctx, tripStatusChange)
	})

	if err != nil {
		return nil, err
	}

	return finalTrip, nil
}

func createTripHash(p CreateTripParams) ([]byte, error) {
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
