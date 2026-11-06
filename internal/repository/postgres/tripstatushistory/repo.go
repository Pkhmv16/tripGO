package tripstatushistory

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PgTripStatusChangeRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewPgTripStatusChangeRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *PgTripStatusChangeRepository {
	return &PgTripStatusChangeRepository{pool: pool, queryTimeout: queryTimeout}
}
