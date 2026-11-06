//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type tripRepository interface {
	Get(context.Context, domaintrip.TripId) (*domaintrip.Trip, error)
	Create(context.Context, *domaintrip.Trip) error
}
type tripStatusChangeRepository interface {
	Save(context.Context, *domaintrip.TripStatusChange) error
}
type idempotencyRepository interface {
	FindForCreate(context.Context, string) (*domaintrip.IdempotencyRecord, error)
	Save(context.Context, string, []byte, domaintrip.TripId) error
}
type txManager interface {
	Do(context.Context, func(context.Context) error) error
}
type idGenerator interface{ Generate() string }

type logger interface {
	InfoContext(context.Context, string, ...any)
	ErrorContext(context.Context, string, ...any)
}
