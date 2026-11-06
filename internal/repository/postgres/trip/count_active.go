package trip

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
)

func (r *PgTripRepository) CountActive(ctx context.Context) (int64, error) {
	query, args, err := squirrel.Select("count(*)").From("trips").
		Where(squirrel.Eq{"status": domaintrip.StatusActive}).
		PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build count active trips query: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var count int64
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
}
