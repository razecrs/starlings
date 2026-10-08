package starlings

import (
	"context"
	"sync"
	"time"
)

// Discord closes a connection that sends more than 120 commands in 60
// seconds. A few of those are held back for heartbeats, identify, and resume,
// so a burst of member or presence requests can never cost the connection.
const (
	gatewaySendLimit    = 120
	gatewaySendReserved = 5
	gatewaySendWindow   = 60 * time.Second
)

// sendWindow counts gateway commands in fixed 60-second windows that start
// when a connection opens, matching how Discord counts them.
type sendWindow struct {
	mu    sync.Mutex
	start time.Time
	used  int
}

func (w *sendWindow) reset(now time.Time) {
	w.mu.Lock()
	w.start, w.used = now, 0
	w.mu.Unlock()
}

// take claims one send. Priority sends may use the reserved slots; other
// sends wait for the next window once only the reserve is left.
func (w *sendWindow) take(ctx context.Context, priority bool) error {
	limit := gatewaySendLimit - gatewaySendReserved
	if priority {
		limit = gatewaySendLimit
	}
	for {
		w.mu.Lock()
		now := time.Now()
		if w.start.IsZero() || now.Sub(w.start) >= gatewaySendWindow {
			w.start, w.used = now, 0
		}
		if w.used < limit {
			w.used++
			w.mu.Unlock()
			return nil
		}
		wait := gatewaySendWindow - now.Sub(w.start)
		w.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
