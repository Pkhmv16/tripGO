package trip

import (
	"time"
)

type (
	TripStatusChangeId int64
	TripStatusChange   struct {
		Id         TripStatusChangeId
		TripId     TripId
		FromStatus *TripStatus
		ToStatus   TripStatus
		Reason     *string
		ChangedAt  time.Time
	}
)

func NewTripStatusChange(
	tripId TripId,
	fromStatus *TripStatus,
	toStatus TripStatus,
	reason *string,
	now time.Time,
) *TripStatusChange {
	change := &TripStatusChange{
		TripId:    tripId,
		ToStatus:  toStatus,
		ChangedAt: now,
	}

	if fromStatus != nil {
		change.FromStatus = new(*fromStatus)
	}

	if reason != nil {
		change.Reason = new(*reason)
	}

	return change
}
