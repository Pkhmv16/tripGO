//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package finish_test
package finish

import (
	"context"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type tripRepository interface {
	GetForUpdate(context.Context, domaintrip.TripId) (*domaintrip.Trip, error)
	Update(context.Context, *domaintrip.Trip) error
}
type tripStatusChangeRepository interface {
	Save(context.Context, *domaintrip.TripStatusChange) error
}
type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type logger interface {
	InfoContext(context.Context, string, ...any)
	ErrorContext(context.Context, string, ...any)
}
