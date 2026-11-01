package trip

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
)

func (r *PgTripRepository) Update(ctx context.Context, trip *domaintrip.Trip) error {
	query, args, err := squirrel.Update("trips").
		Set("status", trip.Status).
		Set("finished_at", trip.FinishedAt).
		Set("updated_at", trip.UpdatedAt).
		Where(squirrel.Eq{"id": trip.Id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update trip query: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	result, err := executor.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update trip: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domaintrip.ErrTripNotFound
	}
	return nil
}
