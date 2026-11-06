package trip_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

func TestNewTripStatusChange(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		fromStatus *trip.TripStatus
		toStatus   trip.TripStatus
		reason     *string
	}{
		{name: "creation without reason", toStatus: trip.StatusActive},
		{name: "creation with reason", toStatus: trip.StatusActive, reason: new("created")},
		{name: "completion without reason", fromStatus: new(trip.StatusActive), toStatus: trip.StatusCompleted},
		{name: "completion with reason", fromStatus: new(trip.StatusActive), toStatus: trip.StatusCompleted, reason: new("finished")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trip.NewTripStatusChange(testTripID, tt.fromStatus, tt.toStatus, tt.reason, now)
			want := &trip.TripStatusChange{
				TripId: testTripID, FromStatus: tt.fromStatus,
				ToStatus: tt.toStatus, Reason: tt.reason, ChangedAt: now,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("NewTripStatusChange() = %+v, want %+v", got, want)
			}

			if tt.fromStatus != nil {
				original := *tt.fromStatus
				*tt.fromStatus = trip.TripStatus("changed")
				if *got.FromStatus != original {
					t.Error("changing the source status changed the history record")
				}
			}
			if tt.reason != nil {
				original := *tt.reason
				*tt.reason = "changed"
				if *got.Reason != original {
					t.Error("changing the source reason changed the history record")
				}
			}
		})
	}
}
