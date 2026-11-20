package trip

import "time"

type Position struct {
	ID         int64
	TripID     TripId
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}
