package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

const lastActivity = "COALESCE(last_position_at, started_at)"

func staleTripsQuery(staleAfter time.Duration, limit uint64, after *domaintrip.PositionCursor) squirrel.SelectBuilder {
	query := squirrel.Select("id", "driver_id", lastActivity).
		From("trips").
		Where(squirrel.Expr("status = 'active'")).
		Where(squirrel.Expr(lastActivity+" < statement_timestamp() - make_interval(secs => ?)", staleAfter.Seconds())).
		OrderBy(lastActivity+" ASC", "id ASC").
		Limit(limit).
		PlaceholderFormat(squirrel.Dollar)
	if after != nil {
		query = query.Where(squirrel.Expr("("+lastActivity+", id) > (?, ?)", after.LastActivity, after.TripID))
	}
	return query
}

func (r *PgTripRepository) ListStaleTrips(ctx context.Context, staleAfter time.Duration, limit int, after *domaintrip.PositionCursor) ([]domaintrip.PositionCandidate, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("stale trip batch size must be positive")
	}
	query, args, err := staleTripsQuery(staleAfter, uint64(limit), after).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list stale trips query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query stale trips: %w", err)
	}
	trips, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domaintrip.PositionCandidate, error) {
		var candidate domaintrip.PositionCandidate
		err := row.Scan(&candidate.TripID, &candidate.DriverID, &candidate.LastActivity)
		return candidate, err
	})
	if err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}
	return trips, nil
}
