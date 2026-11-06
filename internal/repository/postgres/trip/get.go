package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
)

func (r *PgTripRepository) Get(ctx context.Context, tripId domaintrip.TripId) (*domaintrip.Trip, error) {
	return r.get(ctx, tripId, false)
}

func (r *PgTripRepository) GetForUpdate(ctx context.Context, tripId domaintrip.TripId) (*domaintrip.Trip, error) {
	if _, ok := transaction.TxFromContext(ctx); !ok {
		return nil, errors.New("GetForUpdate requires a transaction")
	}
	return r.get(ctx, tripId, true)
}

func (r *PgTripRepository) get(ctx context.Context, tripId domaintrip.TripId, forUpdate bool) (*domaintrip.Trip, error) {
	queryBuilder := squirrel.Select(
		"id",
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
		"updated_at",
	).From("trips").
		PlaceholderFormat(squirrel.Dollar).
		Where(squirrel.Eq{"id": tripId})
	if forUpdate {
		queryBuilder = queryBuilder.Suffix("FOR UPDATE")
	}
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select trip query: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)

	var trip domaintrip.Trip

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	err = executor.QueryRow(ctx, query, args...).Scan(
		&trip.Id,
		&trip.UserId,
		&trip.DriverId,
		&trip.StartLatitude,
		&trip.StartLongitude,
		&trip.EndLatitude,
		&trip.EndLongitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
		&trip.CreatedAt,
		&trip.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domaintrip.ErrTripNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}

	return &trip, nil
}
