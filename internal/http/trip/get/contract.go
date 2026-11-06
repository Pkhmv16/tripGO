//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package get_test
package get

import (
	"context"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type usecaseContract interface {
	GetTrip(ctx context.Context, tripId string) (*trip.Trip, error)
}
