//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package listpositions_test
package listpositions

import (
	"context"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type usecaseContract interface {
	ListPositions(context.Context, trip.TripId) ([]trip.Position, error)
}
