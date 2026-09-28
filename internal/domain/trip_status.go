package domain

type TripStatus string

const (
	StatusActive    TripStatus = "active"
	StatusCompleted TripStatus = "completed"
)

func (t TripStatus) IsValid() bool {
	switch t {
	case StatusActive, StatusCompleted:
		return true
	}
	return false
}
