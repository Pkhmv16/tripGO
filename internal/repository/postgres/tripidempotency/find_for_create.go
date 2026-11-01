package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
	"github.com/jackc/pgx/v5"
)

func (r *PgIdempotencyRepository) FindForCreate(
	ctx context.Context,
	key string,
) (*trip.IdempotencyRecord, error) {
	if _, ok := transaction.TxFromContext(ctx); !ok {
		return nil, errors.New("tripidempotency lookup requires a transaction")
	}

	executor := transaction.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	digest := sha256.Sum256([]byte("trip-create:" + key))
	lockID := int64(binary.BigEndian.Uint64(digest[:8]))

	query, args, err := squirrel.Select().
		Column("pg_advisory_xact_lock(?)", lockID).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build tripidempotency lock: %w", err)
	}

	if _, err := executor.Exec(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("lock tripidempotency key: %w", err)
	}

	query, args, err = squirrel.Delete("trip_idempotency").
		Where(squirrel.Eq{"key": key}).
		Where("expires_at <= clock_timestamp()").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build expired key deletion: %w", err)
	}

	if _, err := executor.Exec(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("delete expired key: %w", err)
	}

	query, args, err = squirrel.Select("request_hash", "trip_id").
		From("trip_idempotency").
		Where(squirrel.Eq{"key": key}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()

	if err != nil {
		return nil, fmt.Errorf("build tripidempotency lookup: %w", err)
	}

	var record trip.IdempotencyRecord

	err = executor.QueryRow(ctx, query, args...).Scan(
		&record.RequestHash,
		&record.TripId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find tripidempotency key: %w", err)
	}

	return &record, nil
}
