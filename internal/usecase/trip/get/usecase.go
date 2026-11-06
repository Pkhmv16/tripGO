package get

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Usecase struct {
	tripRepo tripRepository
	tracer   trace.Tracer
}

func New(repo tripRepository, tracer trace.Tracer) *Usecase {
	return &Usecase{tripRepo: repo, tracer: tracer}
}

func (uc *Usecase) GetTrip(ctx context.Context, tripId string) (*domaintrip.Trip, error) {
	ctx, span := uc.tracer.Start(ctx, "trip.get")
	defer span.End()
	trip, err := uc.tripRepo.Get(ctx, domaintrip.TripId(tripId))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
		return nil, err
	}
	return trip, nil
}
