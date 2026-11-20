package clock

import "time"

type Ticker struct {
	*time.Ticker
}

func NewTicker(interval time.Duration) *Ticker {
	return &Ticker{Ticker: time.NewTicker(interval)}
}

func (t *Ticker) Ticks() <-chan time.Time {
	return t.C
}
