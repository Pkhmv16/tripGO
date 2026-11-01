//go:build integration

package transaction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
	triprepository "github.com/Pkhmv16/tripGO/internal/repository/postgres/trip"
	"github.com/Pkhmv16/tripGO/internal/repository/postgres/tripstatushistory"
	testpostgres "github.com/Pkhmv16/tripGO/internal/testutil/postgres"
)

func TestTxManagerDoRollsBackTripOnHistoryError(t *testing.T) {
	pool := testpostgres.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tripRepo := triprepository.NewPgTripRepository(pool, 3*time.Second)
	historyRepo := tripstatushistory.NewPgTripStatusChangeRepository(pool, 3*time.Second)
	txManager := transaction.NewTxManager(pool, 3*time.Second, 3*time.Second)
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	trip := domaintrip.NewTrip(
		domaintrip.TripId(uuid.NewString()),
		domaintrip.UserId(uuid.NewString()),
		domaintrip.DriverId(uuid.NewString()),
		59.9398, 30.3146, 59.929, 30.3626, 1450, now,
	)

	var historyErr error
	err := txManager.Do(ctx, func(txCtx context.Context) error {
		if err := tripRepo.Create(txCtx, trip); err != nil {
			return err
		}
		inserted, err := tripRepo.Get(txCtx, trip.Id)
		if err != nil {
			return err
		}
		if inserted.Id != trip.Id || inserted.Status != domaintrip.StatusActive {
			t.Errorf("unexpected trip inside transaction: %+v", inserted)
		}

		change := domaintrip.NewTripStatusChange(
			trip.Id, nil, domaintrip.TripStatus("invalid"), nil, now,
		)
		historyErr = historyRepo.Save(txCtx, change)
		return historyErr
	})
	if historyErr == nil {
		t.Fatal("expected history insert to fail")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "trip_status_history_to_status_check" {
		t.Fatalf("expected history status CHECK violation, got %v", err)
	}

	for _, query := range []string{
		"SELECT count(*) FROM trips WHERE id = $1",
		"SELECT count(*) FROM trip_status_history WHERE trip_id = $1",
	} {
		var count int
		if err := pool.QueryRow(ctx, query, trip.Id).Scan(&count); err != nil {
			t.Fatalf("check rollback with %q: %v", query, err)
		}
		if count != 0 {
			t.Errorf("rollback left %d rows for %q", count, query)
		}
	}
	if _, err := tripRepo.Get(ctx, trip.Id); !errors.Is(err, domaintrip.ErrTripNotFound) {
		t.Errorf("Get() after rollback error = %v, want ErrTripNotFound", err)
	}
}
