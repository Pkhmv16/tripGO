package get

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Usecase struct {
	tripRepo tripRepository
	logger   logger
	tracer   trace.Tracer
}

func New(repo tripRepository, log logger, tracer trace.Tracer) *Usecase {
	return &Usecase{tripRepo: repo, logger: log, tracer: tracer}
}

func (uc *Usecase) GetTrip(ctx context.Context, tripId string) (*domaintrip.Trip, error) {
	ctx, span := uc.tracer.Start(ctx, "trip.get")
	defer span.End()
	trip, err := uc.tripRepo.Get(ctx, domaintrip.TripId(tripId))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
		if !errors.Is(err, domaintrip.ErrTripNotFound) {
			uc.logger.ErrorContext(ctx, "trip retrieval failed", "trip_id", tripId, "error", err)
		}
		return nil, err
	}
	return trip, nil
}
