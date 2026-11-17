//go:build integration

package trip_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	triprepository "github.com/Pkhmv16/tripGO/internal/repository/postgres/trip"
	"github.com/Pkhmv16/tripGO/internal/repository/postgres/tripposition"
	testpostgres "github.com/Pkhmv16/tripGO/internal/testutil/postgres"
)

func TestListPositionsOrdersByRecordedAtThenID(t *testing.T) {
	pool := testpostgres.NewPool(t)
	const timeout = 3 * time.Second
	trips := triprepository.NewPgTripRepository(pool, timeout)
	positions := tripposition.New(pool, timeout)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Second)
	ride := domaintrip.NewTrip(domaintrip.TripId(uuid.NewString()), domaintrip.UserId(uuid.NewString()),
		domaintrip.DriverId(uuid.NewString()), 0, 0, 1, 1, 100, now.Add(-time.Hour))
	if err := trips.Create(ctx, ride); err != nil {
		t.Fatal(err)
	}

	recordedTimes := []time.Time{now, now.Add(-time.Minute), now.Add(-time.Minute)}
	inserted := make([]int64, 0, len(recordedTimes))
	for _, recordedAt := range recordedTimes {
		point, err := positions.Save(ctx, domaintrip.Position{TripID: ride.Id, RecordedAt: recordedAt})
		if err != nil {
			t.Fatal(err)
		}
		inserted = append(inserted, point.ID)
	}

	points, err := positions.ListByTrip(ctx, ride.Id)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(points))
	for _, point := range points {
		ids = append(ids, point.ID)
	}
	want := []int64{inserted[1], inserted[2], inserted[0]}
	if !slices.Equal(ids, want) {
		t.Errorf("position IDs = %v, want %v", ids, want)
	}
}
