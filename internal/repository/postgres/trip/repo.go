package trip

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PgTripRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewPgTripRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *PgTripRepository {
	return &PgTripRepository{pool: pool, queryTimeout: queryTimeout}
}
