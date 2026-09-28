package domain

import "errors"

var (
	ErrTripNotFound         = errors.New("trip not found")
	ErrTripAlreadyCompleted = errors.New("trip already completed")
	ErrDriverBusy           = errors.New("driver already has an active trip")
)
