package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
)

func (r *PgTripRepository) Create(ctx context.Context, trip *domaintrip.Trip) error {
	query, args, err := squirrel.Insert("trips").
		PlaceholderFormat(squirrel.Dollar).
		Columns("id",
			"user_id",
			"driver_id",
			"start_latitude",
			"start_longitude",
			"end_latitude",
			"end_longitude",
			"price",
			"status",
			"started_at",
			"finished_at",
			"created_at",
			"updated_at").
		Values(trip.Id,
			trip.UserId,
			trip.DriverId,
			trip.StartLatitude,
			trip.StartLongitude,
			trip.EndLatitude,
			trip.EndLongitude,
			trip.Price,
			trip.Status,
			trip.StartedAt,
			trip.FinishedAt,
			trip.CreatedAt,
			trip.UpdatedAt).ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	_, err = executor.Exec(ctx, query, args...)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "trips_one_active_per_driver_idx" {
		return domaintrip.ErrDriverBusy
	}

	if err != nil {
		return fmt.Errorf("insert trip: %w", err)
	}

	return nil
}
