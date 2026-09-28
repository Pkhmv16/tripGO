package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/Pkhmv16/tripGO/internal/domain"
	"github.com/Pkhmv16/tripGO/internal/infrastructure"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgTripRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewPgTripRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *PgTripRepository {
	return &PgTripRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *PgTripRepository) Get(ctx context.Context, tripId domain.TripId) (*domain.Trip, error) {
	query, args, err := squirrel.Select(
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
		Where(squirrel.Eq{"id": tripId}).
		ToSql()

	if err != nil {
		return nil, fmt.Errorf("build select trip query: %w", err)
	}

	executor := infrastructure.Executor(ctx, r.pool)

	var trip domain.Trip

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
		&trip.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTripNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}

	return &trip, nil
}

func (r *PgTripRepository) Create(ctx context.Context, trip *domain.Trip) error {
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

	executor := infrastructure.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	_, err = executor.Exec(ctx, query, args...)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "trips_one_active_per_driver_idx" {
		return domain.ErrDriverBusy
	}

	if err != nil {
		return fmt.Errorf("insert trip: %w", err)
	}

	return nil
}

func (r *PgTripRepository) Finish(ctx context.Context, tripId domain.TripId, finishedAt time.Time) (bool, error) {
	query, args, err := squirrel.Update("trips").
		PlaceholderFormat(squirrel.Dollar).
		Set("status", domain.StatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{"id": tripId}).
		Where(squirrel.Eq{"status": domain.StatusActive}).ToSql()

	if err != nil {
		return false, fmt.Errorf("build update finish trip: %w", err)
	}

	executor := infrastructure.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	result, err := executor.Exec(ctx, query, args...)

	if err != nil {
		err = fmt.Errorf("finish trip: %w", err)
	}

	return result.RowsAffected() > 0, err
}
