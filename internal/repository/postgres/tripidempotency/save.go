package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
)

func (r *PgIdempotencyRepository) Save(
	ctx context.Context,
	key string,
	requestHash []byte,
	tripId trip.TripId,
) error {
	if _, ok := transaction.TxFromContext(ctx); !ok {
		return errors.New(
			"saving tripidempotency key requires a transaction",
		)
	}

	query, args, err := squirrel.Insert("trip_idempotency").
		Columns("key", "request_hash", "trip_id", "expires_at").
		Values(
			key,
			requestHash,
			tripId,
			squirrel.Expr("clock_timestamp() + ?::bigint * interval '1 microsecond'", r.ttl.Microseconds())).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()

	if err != nil {
		return fmt.Errorf("build tripidempotency insert: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := executor.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("save tripidempotency key: %w", err)
	}

	return nil
}
