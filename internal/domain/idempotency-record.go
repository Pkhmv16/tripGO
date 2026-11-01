package domain

type IdempotencyRecord struct {
	RequestHash []byte
	TripId      TripId
}
