//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package listpositions_test
package listpositions

import (
	"context"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type tripRepository interface {
	Get(context.Context, trip.TripId) (*trip.Trip, error)
}

type positionRepository interface {
	ListByTrip(context.Context, trip.TripId) ([]trip.Position, error)
}

type logger interface {
	ErrorContext(context.Context, string, ...any)
}
