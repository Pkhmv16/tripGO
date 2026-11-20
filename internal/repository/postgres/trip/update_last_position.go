package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
)

func (r *PgTripRepository) UpdateLastPosition(ctx context.Context, id domaintrip.TripId, recordedAt time.Time) error {
	query, args, err := squirrel.Update("trips").
		Set("last_position_at", squirrel.Expr("GREATEST(last_position_at, ?)", recordedAt)).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update last position time: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	result, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update last position time: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domaintrip.ErrTripNotFound
	}
	return nil
}
