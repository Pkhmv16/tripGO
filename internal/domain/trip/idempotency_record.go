package trip

type IdempotencyRecord struct {
	RequestHash []byte
	TripId      TripId
}
