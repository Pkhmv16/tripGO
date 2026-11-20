package trip

import "time"

type PositionCandidate struct {
	TripID       TripId
	DriverID     DriverId
	LastActivity time.Time
}

type PositionCursor struct {
	LastActivity time.Time
	TripID       TripId
}
