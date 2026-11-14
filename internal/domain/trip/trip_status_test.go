package trip_test

import (
	"testing"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

func TestTripStatusIsValid(t *testing.T) {
	tests := []struct {
		name   string
		status trip.TripStatus
		want   bool
	}{
		{name: "active", status: trip.StatusActive, want: true},
		{name: "completed", status: trip.StatusCompleted, want: true},
		{name: "empty"},
		{name: "unknown", status: trip.TripStatus("cancelled")},
		{name: "wrong case", status: trip.TripStatus("ACTIVE")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("IsValid(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
