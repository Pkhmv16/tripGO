//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package get_test
package get

import (
	"context"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type tripRepository interface {
	Get(context.Context, domaintrip.TripId) (*domaintrip.Trip, error)
}
