package service

import (
	"context"
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

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type IdGenerator interface {
	Generate() string
}

type TripService struct {
	tripRepo             TripRepo
	tripStatusChangeRepo TripStatusChangeRepo
	txManager            TxManager
	idGenerator          IdGenerator
}

func NewTripService(
	tripRepo TripRepo,
	tripStatusChangeRepo TripStatusChangeRepo,
	txManager TxManager,
	idGenerator IdGenerator) *TripService {
	return &TripService{
		tripRepo:             tripRepo,
		tripStatusChangeRepo: tripStatusChangeRepo,
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
}

func (ts *TripService) CreateTrip(ctx context.Context, p CreateTripParams) (*domain.Trip, error) {
	tripId := domain.TripId(ts.idGenerator.Generate())
	now := time.Now()
	trip := domain.NewTrip(
		tripId,
		p.UserId,
		p.DriverId,
		p.StartLatitude,
		p.StartLongitude,
		p.EndLatitude,
		p.EndLongitude,
		p.Price,
		now)

	err := ts.txManager.Do(ctx, func(ctx context.Context) error {
		if err := ts.tripRepo.Create(ctx, trip); err != nil {
			return err
		}
		tripStatusChange := domain.NewTripStatusChange(
			tripId,
			nil,
			"active",
			new("Trip initialized by service"),
			now)

		return ts.tripStatusChangeRepo.Save(ctx, tripStatusChange)
	})

	if err != nil {
		return nil, err
	}

	return trip, nil
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
