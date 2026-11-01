package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/Pkhmv16/tripGO/internal/domain"
	"github.com/Pkhmv16/tripGO/internal/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgTripStatusChangeRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewPgTripStatusChangeRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *PgTripStatusChangeRepository {
	return &PgTripStatusChangeRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *PgTripStatusChangeRepository) Save(ctx context.Context, tripStatusChange *domain.TripStatusChange) error {
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

	executor := infrastructure.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := executor.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip_status_history: %w", err)
	}

	return nil
}
