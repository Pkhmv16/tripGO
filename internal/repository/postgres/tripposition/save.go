package tripposition

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
)

type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func New(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}

func (r *Repository) Save(ctx context.Context, position trip.Position) (trip.Position, error) {
	query, args, err := squirrel.Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(position.TripID, position.Latitude, position.Longitude, position.RecordedAt).
		Suffix("RETURNING id, recorded_at").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return trip.Position{}, fmt.Errorf("build insert trip position: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	err = transaction.Executor(ctx, r.pool).QueryRow(ctx, query, args...).
		Scan(&position.ID, &position.RecordedAt)
	if err != nil {
		return trip.Position{}, fmt.Errorf("insert trip position: %w", err)
	}
	return position, nil
}
