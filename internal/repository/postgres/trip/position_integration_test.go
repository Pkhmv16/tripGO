//go:build integration

package trip_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	"github.com/Pkhmv16/tripGO/internal/infra/postgres/transaction"
	triprepository "github.com/Pkhmv16/tripGO/internal/repository/postgres/trip"
	"github.com/Pkhmv16/tripGO/internal/repository/postgres/tripposition"
	testpostgres "github.com/Pkhmv16/tripGO/internal/testutil/postgres"
	"github.com/Pkhmv16/tripGO/internal/usecase/trip/saveposition"
)

func TestSavePositionKeepsLatestTimestampAndRollsBack(t *testing.T) {
	pool := testpostgres.NewPool(t)
	const timeout = 3 * time.Second
	repo := triprepository.NewPgTripRepository(pool, timeout)
	uc := saveposition.New(repo, tripposition.New(pool, timeout), transaction.NewTxManager(pool, timeout, timeout),
		slog.New(slog.DiscardHandler), noop.Int64Counter{})
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
	assertStoredPositions(t, ctx, repo, pool, ride.Id, now, 2)

	if _, err := pool.Exec(ctx, "ALTER TABLE trips ADD CONSTRAINT reject_position_update CHECK (last_position_at IS NULL) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.SavePosition(ctx, domaintrip.Position{TripID: ride.Id, RecordedAt: now.Add(time.Second)}); err == nil {
		t.Fatal("expected last_position_at update to fail")
	}
	assertStoredPositions(t, ctx, repo, pool, ride.Id, now, 2)
}

func assertStoredPositions(t *testing.T, ctx context.Context, repo *triprepository.PgTripRepository, db transaction.DBTX, id domaintrip.TripId, latest time.Time, wantCount int) {
	t.Helper()
	ride, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ride.LastPositionAt == nil || !ride.LastPositionAt.Equal(latest) {
		t.Errorf("last_position_at = %v, want %v", ride.LastPositionAt, latest)
	}
	query, args, err := squirrel.Select("count(*)").
		From("trip_positions").
		Where(squirrel.Eq{"trip_id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != wantCount {
		t.Errorf("positions count = %d, want %d", count, wantCount)
	}
}
