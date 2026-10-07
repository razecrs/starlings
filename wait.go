package starlings

import (
	"context"
	"sync"
)

type eventResult[E Event] struct {
	event *E
	err   error
}

// EventWaiter owns a one-event subscription. Wait consumes the result once;
// Close and context cancellation remove the subscription. It does not retain
// a growing event history.
type EventWaiter[E Event] struct {
	result chan eventResult[E]
	cancel context.CancelFunc
}

// Watch subscribes immediately to the next matching event. Register before
// sending a prompt, then Wait outside a synchronous gateway handler. Match
// should be quick and side-effect free, and check user/channel IDs for private
// workflows. A nil predicate accepts any event of E.
//
//	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
//	defer cancel()
//	reply := starlings.Watch[starlings.MessageCreate](ctx, bot, func(m *starlings.MessageCreate) bool {
//		return m.ChannelID == channelID && m.Author != nil && m.Author.ID == userID
//	})
//	defer reply.Close()
//	// Send the prompt now, after the subscription exists.
//	message, err := reply.Wait()
func Watch[E Event](ctx context.Context, c *Client, match func(*E) bool) *EventWaiter[E] {
	ctx, cancel := context.WithCancel(ctx)
	w := &EventWaiter[E]{result: make(chan eventResult[E], 1), cancel: cancel}
	var once sync.Once
	var off func()
	registered := make(chan struct{})
	finish := func(value *E, err error) {
		once.Do(func() {
			<-registered
			off()
			w.result <- eventResult[E]{value, err}
			close(w.result)
			cancel()
		})
	}
	off = Listen(c, func(event *E) {
		if ctx.Err() != nil {
			return
		}
		if match == nil || match(event) {
			finish(event, nil)
		}
	})
	close(registered)
	context.AfterFunc(ctx, func() { finish(nil, ctx.Err()) })
	return w
}

// Wait waits for one matching event or cancellation. Call it once per waiter.
func (w *EventWaiter[E]) Wait() (*E, error) {
	result, ok := <-w.result
	if !ok {
		return nil, ErrClosed
	}
	return result.event, result.err
}

// Close cancels an unfinished wait and removes its listener.
func (w *EventWaiter[E]) Close() { w.cancel() }
