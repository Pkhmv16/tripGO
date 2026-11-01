package trip

import "errors"

var (
	ErrTripNotFound         = errors.New("trip not found")
	ErrTripAlreadyCompleted = errors.New("trip already completed")
	ErrDriverBusy           = errors.New("driver already has an active trip")
	ErrIdempotencyConflict  = errors.New("tripidempotency key was already used with a different request")
)
