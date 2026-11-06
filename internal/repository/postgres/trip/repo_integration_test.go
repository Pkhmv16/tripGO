//go:build integration

package trip_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"
	triprepository "github.com/Pkhmv16/tripGO/internal/repository/postgres/trip"
	testpostgres "github.com/Pkhmv16/tripGO/internal/testutil/postgres"
)

func TestRepositoryCreateAndGet(t *testing.T) {
	pool := testpostgres.NewPool(t)
	repo := triprepository.NewPgTripRepository(pool, 3*time.Second)

	for _, status := range []domaintrip.TripStatus{domaintrip.StatusActive, domaintrip.StatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			now := time.Date(2026, 11, 1, 10, 0, 0, 123456000, time.UTC)
			want := domaintrip.NewTrip(
				domaintrip.TripId(uuid.NewString()),
				domaintrip.UserId(uuid.NewString()),
				domaintrip.DriverId(uuid.NewString()),
				59.9398, 30.3146, -45.125, 120.875, 1450, now,
			)
			if status == domaintrip.StatusCompleted {
				if err := want.Finish(now.Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.Create(ctx, want); err != nil {
				t.Fatalf("Create(): %v", err)
			}

			var storedCorrectly bool
			err := pool.QueryRow(ctx, `
				SELECT user_id = $2 AND driver_id = $3
				    AND start_latitude = $4 AND start_longitude = $5
				    AND end_latitude = $6 AND end_longitude = $7
				    AND price = $8 AND status = $9
				    AND started_at = $10 AND created_at = $11 AND updated_at = $12
				    AND finished_at IS NOT DISTINCT FROM $13
				FROM trips WHERE id = $1`,
				want.Id, want.UserId, want.DriverId,
				want.StartLatitude, want.StartLongitude, want.EndLatitude, want.EndLongitude,
				want.Price, want.Status, want.StartedAt, want.CreatedAt, want.UpdatedAt, want.FinishedAt,
			).Scan(&storedCorrectly)
			if err != nil {
				t.Fatalf("read stored fields directly: %v", err)
			}
			if !storedCorrectly {
				t.Fatal("stored fields differ from the created trip")
			}

			got, err := repo.Get(ctx, want.Id)
			if err != nil {
				t.Fatalf("Get(): %v", err)
			}
			got.StartedAt = got.StartedAt.UTC()
			got.CreatedAt = got.CreatedAt.UTC()
			got.UpdatedAt = got.UpdatedAt.UTC()
			if got.FinishedAt != nil {
				got.FinishedAt = new(got.FinishedAt.UTC())
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Get() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestRepositoryConcurrentCreateForSameDriver(t *testing.T) {
	pool := testpostgres.NewPool(t)
	repo := triprepository.NewPgTripRepository(pool, 3*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	driverID := domaintrip.DriverId(uuid.NewString())

	type result struct {
		id  domaintrip.TripId
		err error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for range 2 {
		candidate := domaintrip.NewTrip(
			domaintrip.TripId(uuid.NewString()), domaintrip.UserId(uuid.NewString()), driverID,
			59.9398, 30.3146, 59.929, 30.3626, 1450, time.Now().UTC(),
		)
		go func() {
			<-start
			results <- result{id: candidate.Id, err: repo.Create(ctx, candidate)}
		}()
	}
	close(start)

	var winner, rejected domaintrip.TripId
	succeeded, busy := 0, 0
	for range 2 {
		select {
		case got := <-results:
			switch {
			case got.err == nil:
				winner = got.id
				succeeded++
			case errors.Is(got.err, domaintrip.ErrDriverBusy):
				rejected = got.id
				busy++
			default:
				t.Errorf("unexpected Create() error: %v", got.err)
			}
		case <-ctx.Done():
			t.Fatalf("concurrent creation did not finish: %v", ctx.Err())
		}
	}
	if succeeded != 1 || busy != 1 {
		t.Fatalf("successes = %d, driver busy errors = %d; want 1 each", succeeded, busy)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trips WHERE driver_id = $1", driverID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("stored trips for driver = %d, want 1", count)
	}
	if got, err := repo.Get(ctx, winner); err != nil || got.Id != winner {
		t.Errorf("winning trip was not saved: got %+v, error %v", got, err)
	}
	if _, err := repo.Get(ctx, rejected); !errors.Is(err, domaintrip.ErrTripNotFound) {
		t.Errorf("rejected trip Get() error = %v, want ErrTripNotFound", err)
	}
}
