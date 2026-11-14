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

	active := *trip.NewTrip(testTripID, "user-id", "driver-id", 59, 30, 60, 31, 1450, startedAt)
	completed := active
	completed.Status = trip.StatusCompleted
	completed.FinishedAt = &previousFinish
	unknown := active
	unknown.Status = "cancelled"
	empty := active
	empty.Status = ""
	wantCompleted := active
	wantCompleted.Status = trip.StatusCompleted
	wantCompleted.FinishedAt = &finishedAt
	wantCompleted.UpdatedAt = finishedAt

	tests := []struct {
		name        string
		input, want trip.Trip
		finishAt    time.Time
		wantErr     error
	}{
		{name: "active trip", input: active, want: wantCompleted, finishAt: finishedAt},
		{name: "completed trip", input: completed, want: completed, finishAt: finishedAt, wantErr: trip.ErrTripAlreadyCompleted},
		{name: "unknown status", input: unknown, want: unknown, finishAt: finishedAt, wantErr: trip.ErrTripAlreadyCompleted},
		{name: "empty status", input: empty, want: empty, finishAt: finishedAt, wantErr: trip.ErrTripAlreadyCompleted},
		{name: "repeated finish", input: wantCompleted, want: wantCompleted, finishAt: finishedAt.Add(time.Hour), wantErr: trip.ErrTripAlreadyCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input
			if err := got.Finish(tt.finishAt); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Finish() trip = %+v, want %+v", got, tt.want)
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
