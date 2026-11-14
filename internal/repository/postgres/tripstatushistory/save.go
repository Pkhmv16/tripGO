package tripstatushistory

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
)

func (r *PgTripStatusChangeRepository) Save(ctx context.Context, tripStatusChange *trip.TripStatusChange) error {
	query, args, err := squirrel.Insert("trip_status_history").
		PlaceholderFormat(squirrel.Dollar).
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(tripStatusChange.TripId,
			tripStatusChange.FromStatus,
			tripStatusChange.ToStatus,
			tripStatusChange.Reason,
			tripStatusChange.ChangedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip_status_history: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := executor.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip_status_history: %w", err)
	}

	return nil
}
