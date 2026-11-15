//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package saveposition_test
package saveposition

import (
	"context"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type usecaseContract interface {
	SavePosition(context.Context, trip.Position) (trip.Position, error)
}
