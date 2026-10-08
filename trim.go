package starlings

import (
	"runtime/debug"
	"sync/atomic"
	"time"
)

// WithMemoryTrim controls whether the client returns memory to the operating
// system once the startup burst is over. Connecting delivers every guild and
// member list at once; the Go runtime keeps the memory that burst needed
// unless it is asked to give it back. With trimming on, the default, the
// client does that once, after guild and member events have been quiet for
// two seconds. It costs one garbage collection.
func WithMemoryTrim(enabled bool) Option {
	return func(c *Client) { c.trimMemory = enabled }
}

const (
	trimQuiet   = 2 * time.Second
	trimTimeout = 2 * time.Minute
)

// noteStartupBurst records guild and member traffic. The gateway calls it
// with the event name only, so these large payloads are not decoded for it.
func (c *Client) noteStartupBurst(name string) {
	if name == "GUILD_CREATE" || name == "GUILD_MEMBERS_CHUNK" {
		c.rootClient().trimLast.Store(time.Now().UnixNano())
	}
}

func (c *Client) installMemoryTrim() {
	last := &c.trimLast
	var started atomic.Bool
	internalOn(c, func(*Ready) {
		last.Store(time.Now().UnixNano())
		if !started.CompareAndSwap(false, true) {
			return
		}
		go func() {
			deadline := time.Now().Add(trimTimeout)
			for time.Now().Before(deadline) {
				time.Sleep(trimQuiet / 4)
				if time.Since(time.Unix(0, last.Load())) >= trimQuiet {
					break
				}
			}
			debug.FreeOSMemory()
		}()
	})
}
