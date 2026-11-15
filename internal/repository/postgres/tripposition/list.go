package tripposition

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
)

func (r *Repository) ListByTrip(ctx context.Context, id trip.TripId) ([]trip.Position, error) {
	query, args, err := squirrel.Select("id", "latitude", "longitude", "recorded_at").
		From("trip_positions").
		Where(squirrel.Eq{"trip_id": id}).
		OrderBy("recorded_at ASC", "id ASC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list trip positions: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := transaction.Executor(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query trip positions: %w", err)
	}
	positions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (trip.Position, error) {
		position := trip.Position{TripID: id}
		err := row.Scan(&position.ID, &position.Latitude, &position.Longitude, &position.RecordedAt)
		return position, err
	})
	if err != nil {
		return nil, fmt.Errorf("read trip positions: %w", err)
	}
	return positions, nil
}
