package finish

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Usecase struct {
	completedCounter     metric.Int64Counter
	tracer               trace.Tracer
	logger               logger
	tripRepo             tripRepository
	tripStatusChangeRepo tripStatusChangeRepository
	txManager            txManager
}

func New(repo tripRepository, history tripStatusChangeRepository, tx txManager, log logger, tracer trace.Tracer, completedCounter metric.Int64Counter) *Usecase {
	return &Usecase{tripRepo: repo, tripStatusChangeRepo: history, txManager: tx, logger: log, tracer: tracer, completedCounter: completedCounter}
}

func (uc *Usecase) FinishTrip(ctx context.Context, tripId string) (result *domaintrip.Trip, err error) {
	ctx, span := uc.tracer.Start(ctx, "trip.finish")
	defer span.End()
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
		}
	}()
	var finalTrip *domaintrip.Trip
	domainTripId := domaintrip.TripId(tripId)

	err = uc.txManager.Do(ctx, func(ctx context.Context) error {
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
		if !errors.Is(err, domaintrip.ErrTripNotFound) && !errors.Is(err, domaintrip.ErrTripAlreadyCompleted) {
			uc.logger.ErrorContext(ctx, "trip completion failed", "trip_id", tripId, "error", err)
		}
		return nil, err
	}
	uc.completedCounter.Add(ctx, 1)
	uc.logger.InfoContext(ctx, "trip completed", "trip_id", tripId)
	return finalTrip, nil
}
