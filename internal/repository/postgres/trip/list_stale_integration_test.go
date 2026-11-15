//go:build integration

package trip

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	testpostgres "github.com/Pkhmv16/tripGO/internal/testutil/postgres"
)

func TestListStaleTrips(t *testing.T) {
	pool := testpostgres.NewPool(t)
	repo := NewPgTripRepository(pool, 3*time.Second)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Second)
	const staleAfter = 5 * time.Minute
	tests := []struct {
		name       string
		startedAt  time.Time
		positionAt *time.Time
		finishedAt *time.Time
	}{
		{name: "stale without positions", startedAt: now.Add(-20 * time.Minute)},
		{name: "stale with a position", startedAt: now.Add(-time.Hour), positionAt: new(now.Add(-10 * time.Minute))},
		{name: "fresh with a position", startedAt: now.Add(-time.Hour), positionAt: new(now.Add(-time.Minute))},
		{name: "fresh without positions", startedAt: now.Add(-time.Minute)},
		{name: "completed", startedAt: now.Add(-time.Hour), finishedAt: new(now.Add(-30 * time.Minute))},
	}
	candidates := make(map[string]domaintrip.PositionCandidate, len(tests))
	for _, tt := range tests {
		ride := domaintrip.NewTrip(domaintrip.TripId(uuid.NewString()), domaintrip.UserId(uuid.NewString()),
			domaintrip.DriverId(uuid.NewString()), 0, 0, 1, 1, 100, tt.startedAt)
		if tt.finishedAt != nil {
			if err := ride.Finish(*tt.finishedAt); err != nil {
				t.Fatal(err)
			}
		}
		if err := repo.Create(ctx, ride); err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if tt.positionAt != nil {
			if err := repo.UpdateLastPosition(ctx, ride.Id, *tt.positionAt); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
		}
		candidates[tt.name] = domaintrip.PositionCandidate{TripID: ride.Id, DriverID: ride.DriverId}
	}

	got, err := repo.ListStaleTrips(ctx, staleAfter)
	if err != nil {
		t.Fatal(err)
	}
	want := []domaintrip.PositionCandidate{candidates[tests[0].name], candidates[tests[1].name]}
	if !slices.Equal(got, want) {
		t.Fatalf("stale trips = %+v, want %+v", got, want)
	}

	assertStaleTripsIndexScan(t, ctx, pool, staleAfter)
}

func assertStaleTripsIndexScan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, staleAfter time.Duration) {
	t.Helper()
	query, args, err := squirrel.Insert("trips").
		Columns("id", "user_id", "driver_id", "start_latitude", "start_longitude", "end_latitude", "end_longitude", "price", "status", "started_at").
		Select(squirrel.Select("gen_random_uuid()", "gen_random_uuid()", "gen_random_uuid()",
			"0", "0", "1", "1", "100", "'active'", "statement_timestamp()").From("generate_series(1, 5000)")).
		ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE trips"); err != nil {
		t.Fatal(err)
	}

	query, args, err = staleTripsQuery(staleAfter).Prefix("EXPLAIN (ANALYZE, BUFFERS)").ToSql()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(lines, "\n")
	t.Logf("PostgreSQL plan:\n%s", plan)
	if !strings.Contains(plan, "Index Scan using trips_active_last_activity_idx") {
		t.Fatal("expected PostgreSQL to use the partial index")
	}
}
