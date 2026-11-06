package model

import domaintrip "github.com/Pkhmv16/tripGO/internal/domain/trip"

type CreateTripParams struct {
	UserId         domaintrip.UserId
	DriverId       domaintrip.DriverId
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	IdempotencyKey *string `json:"-"`
}
