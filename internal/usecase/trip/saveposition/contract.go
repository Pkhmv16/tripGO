//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package saveposition_test
package saveposition

import (
	"context"
	"time"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type tripRepository interface {
	GetForUpdate(context.Context, trip.TripId) (*trip.Trip, error)
	UpdateLastPosition(context.Context, trip.TripId, time.Time) error
}

type positionRepository interface {
	Save(context.Context, trip.Position) (trip.Position, error)
}

type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type positionRequestQueue interface {
	Cancel(trip.TripId)
}

type logger interface {
	ErrorContext(context.Context, string, ...any)
}
