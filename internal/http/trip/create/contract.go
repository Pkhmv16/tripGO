//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/model"
)

type usecaseContract interface {
	CreateTrip(ctx context.Context, params model.CreateTripParams) (*trip.Trip, bool, error)
}
