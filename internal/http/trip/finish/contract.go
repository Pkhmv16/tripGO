//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package finish_test
package finish

import (
	"context"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type usecaseContract interface {
	FinishTrip(ctx context.Context, tripId string) (*trip.Trip, error)
}
