package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/Pkhmv16/tripGO/internal/domain"
	"github.com/Pkhmv16/tripGO/internal/infrastructure"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgIdempotencyRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
	ttl          time.Duration
}

func NewPgIdempotencyRepository(
	pool *pgxpool.Pool,
	queryTimeout time.Duration,
	ttl time.Duration,
) *PgIdempotencyRepository {
	return &PgIdempotencyRepository{
		pool:         pool,
		queryTimeout: queryTimeout,
		ttl:          ttl,
	}
}

func (r *PgIdempotencyRepository) FindForCreate(
	ctx context.Context,
	key string,
) (*domain.IdempotencyRecord, error) {
	if _, ok := infrastructure.TxFromContext(ctx); !ok {
		return nil, errors.New("idempotency lookup requires a transaction")
	}

	executor := infrastructure.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	digest := sha256.Sum256([]byte("trip-create:" + key))
	lockID := int64(binary.BigEndian.Uint64(digest[:8]))

	query, args, err := squirrel.Select().
		Column("pg_advisory_xact_lock(?)", lockID).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build idempotency lock: %w", err)
	}

	if _, err := executor.Exec(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("lock idempotency key: %w", err)
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
		return nil, fmt.Errorf("build idempotency lookup: %w", err)
	}

	var record domain.IdempotencyRecord

	err = executor.QueryRow(ctx, query, args...).Scan(
		&record.RequestHash,
		&record.TripId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find idempotency key: %w", err)
	}

	return &record, nil
}

func (r *PgIdempotencyRepository) Save(
	ctx context.Context,
	key string,
	requestHash []byte,
	tripId domain.TripId,
) error {
	if _, ok := infrastructure.TxFromContext(ctx); !ok {
		return errors.New(
			"saving idempotency key requires a transaction",
		)
	}

	query, args, err := squirrel.Insert("trip_idempotency").
		Columns("key", "request_hash", "trip_id", "expires_at").
		Values(
			key,
			requestHash,
			tripId,
			squirrel.Expr("clock_timestamp() + ?::bigint * interval '1 microsecond'", r.ttl.Microseconds())).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()

	if err != nil {
		return fmt.Errorf("build idempotency insert: %w", err)
	}

	executor := infrastructure.Executor(ctx, r.pool)

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := executor.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("save idempotency key: %w", err)
	}

	return nil
}
