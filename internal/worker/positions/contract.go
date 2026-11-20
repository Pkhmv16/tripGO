//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package positions_test
package positions

import (
	"context"
	"time"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type staleTripRepository interface {
	ListStaleTrips(context.Context, time.Duration, int, *trip.PositionCursor) ([]trip.PositionCandidate, error)
}

type queue interface {
	TryEnqueue(context.Context, trip.PositionCandidate) EnqueueResult
}

type ticker interface {
	Ticks() <-chan time.Time
	Stop()
}

type pushGateway interface {
	RequestPosition(context.Context, trip.PositionCandidate) error
}

type EnqueueResult string

const (
	Queued    EnqueueResult = "queued"
	Duplicate EnqueueResult = "duplicate"
	Dropped   EnqueueResult = "dropped"
)
