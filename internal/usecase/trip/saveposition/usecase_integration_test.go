//go:build integration

package saveposition_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
	triprepository "github.com/Pkhmv16/tripGO/internal/repository/postgres/trip"
	"github.com/Pkhmv16/tripGO/internal/repository/postgres/tripposition"
	testpostgres "github.com/Pkhmv16/tripGO/internal/testutil/postgres"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/saveposition"
	positionworker "github.com/Pkhmv16/tripGO/internal/worker/positions"
)

func TestSavePositionKeepsLatestTimestampAndRollsBack(t *testing.T) {
	pool := testpostgres.NewPool(t)
	const timeout = 3 * time.Second
	repo := triprepository.NewPgTripRepository(pool, timeout)
	positions := tripposition.New(pool, timeout)
	uc := saveposition.New(repo, positions, transaction.NewTxManager(pool, timeout, timeout),
		positionworker.NewQueue(1), slog.New(slog.DiscardHandler), noop.Int64Counter{})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Second)
	ride := domaintrip.NewTrip(domaintrip.TripId(uuid.NewString()), domaintrip.UserId(uuid.NewString()),
		domaintrip.DriverId(uuid.NewString()), 0, 0, 1, 1, 100, now.Add(-time.Hour))
	if err := repo.Create(ctx, ride); err != nil {
		t.Fatal(err)
	}

	for _, recordedAt := range []time.Time{now, now.Add(-time.Minute)} {
		position, err := uc.SavePosition(ctx, domaintrip.Position{TripID: ride.Id, RecordedAt: recordedAt})
		if err != nil {
			t.Fatal(err)
		}
		if position.ID <= 0 || !position.RecordedAt.Equal(recordedAt) {
			t.Fatalf("unexpected saved position: %+v", position)
		}
	}
	assertStoredPositions(t, ctx, repo, positions, ride.Id, now, 2)

	if _, err := pool.Exec(ctx, "ALTER TABLE trips ADD CONSTRAINT reject_position_update CHECK (last_position_at IS NULL) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.SavePosition(ctx, domaintrip.Position{TripID: ride.Id, RecordedAt: now.Add(time.Second)}); err == nil {
		t.Fatal("expected last_position_at update to fail")
	}
	assertStoredPositions(t, ctx, repo, positions, ride.Id, now, 2)
}

func assertStoredPositions(t *testing.T, ctx context.Context, repo *triprepository.PgTripRepository, positions *tripposition.Repository, id domaintrip.TripId, latest time.Time, wantCount int) {
	t.Helper()
	ride, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ride.LastPositionAt == nil || !ride.LastPositionAt.Equal(latest) {
		t.Errorf("last_position_at = %v, want %v", ride.LastPositionAt, latest)
	}
	points, err := positions.ListByTrip(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != wantCount {
		t.Errorf("positions count = %d, want %d", len(points), wantCount)
	}
}
