package idempotency

import (
	"time"

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
