package trip_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

const (
	testTripID = "trip-id"
)

func TestTripFinish(t *testing.T) {
	startedAt := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Hour)
	previousFinish := startedAt.Add(30 * time.Minute)

	tests := []struct {
		name       string
		status     trip.TripStatus
		finishedAt *time.Time
		wantErr    error
	}{
		{name: "active trip", status: trip.StatusActive},
		{name: "completed trip", status: trip.StatusCompleted, finishedAt: &previousFinish, wantErr: trip.ErrTripAlreadyCompleted},
		{name: "unknown status", status: trip.TripStatus("cancelled"), wantErr: trip.ErrTripAlreadyCompleted},
		{name: "empty status", wantErr: trip.ErrTripAlreadyCompleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trip.Trip{
				Id: testTripID, UserId: "user-id", DriverId: "driver-id",
				StartLatitude: 59, StartLongitude: 30,
				EndLatitude: 60, EndLongitude: 31,
				Price: 1450, Status: tt.status,
				StartedAt: startedAt, FinishedAt: tt.finishedAt,
				CreatedAt: startedAt, UpdatedAt: startedAt,
			}
			want := got
			if tt.wantErr == nil {
				want.Status = trip.StatusCompleted
				want.FinishedAt = &finishedAt
				want.UpdatedAt = finishedAt
			}

			if err := got.Finish(finishedAt); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Finish() trip = %+v, want %+v", got, want)
			}

			if tt.wantErr == nil {
				if err := got.Finish(finishedAt.Add(time.Hour)); !errors.Is(err, trip.ErrTripAlreadyCompleted) {
					t.Fatalf("repeated Finish() error = %v, want ErrTripAlreadyCompleted", err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("repeated Finish() changed trip: got %+v, want %+v", got, want)
				}
			}
		})
	}
}

func TestNewTrip(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name           string
		startLatitude  float64
		startLongitude float64
		endLatitude    float64
		endLongitude   float64
		price          int64
	}{
		{name: "paid trip", startLatitude: 59.9398, startLongitude: 30.3146, endLatitude: 59.929, endLongitude: 30.3626, price: 1450},
		{name: "free trip at coordinate boundaries", startLatitude: -90, startLongitude: -180, endLatitude: 90, endLongitude: 180},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trip.NewTrip(testTripID, "user-id", "driver-id",
				tt.startLatitude, tt.startLongitude, tt.endLatitude, tt.endLongitude, tt.price, now)
			want := &trip.Trip{
				Id: testTripID, UserId: "user-id", DriverId: "driver-id",
				StartLatitude: tt.startLatitude, StartLongitude: tt.startLongitude,
				EndLatitude: tt.endLatitude, EndLongitude: tt.endLongitude,
				Price: tt.price, Status: trip.StatusActive,
				StartedAt: now, CreatedAt: now, UpdatedAt: now,
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("NewTrip() = %+v, want %+v", got, want)
			}
		})
	}
}
