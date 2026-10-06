package starlings

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// These are written to be run under -race. They target the places where
// starlings deliberately trades locking for speed, because those are exactly
// the places where a data race would hide.

// TestStressDispatchWhileRegistering hammers the copy-on-write handler map:
// the gateway reads it with no lock at all, so registration replacing it
// wholesale has to be safe against concurrent readers.
func TestStressDispatchWhileRegistering(t *testing.T) {
	c := testClient()

	const readers = 8
	var (
		wg        sync.WaitGroup
		delivered atomic.Int64
		stop      atomic.Bool
	)

	On(c, func(*MessageCreate) { delivered.Add(1) })

	// Each reader owns its own gateway, mirroring reality: one read loop per
	// connection, sharing the Client.
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var g gateway
			frame := []byte(messageCreateFrame)
			ctx := context.Background()
			for !stop.Load() {
				if err := g.handleFrame(ctx, c, frame); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}

	// Meanwhile keep replacing the handler map.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			On(c, func(*MessageCreate) { delivered.Add(1) })
			On(c, func(*GuildCreate) {})
			c.Command("cmd", func(*MessageCreate, []string) {})
			c.Slash("s", "d", func(*InteractionCreate) {})
		}
	}()

	time.Sleep(150 * time.Millisecond)
	stop.Store(true)
	wg.Wait()

	if delivered.Load() == 0 {
		t.Fatal("no events were delivered during the stress run")
	}
	t.Logf("delivered %d events across %d readers while registering", delivered.Load(), readers)
}

// TestStressRateLimiter drives the bucket accounting from many goroutines,
// which is how it behaves when a bot fans out requests.
func TestStressRateLimiter(t *testing.T) {
	l := newLimiter()
	ctx := context.Background()

	const goroutines = 32
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Half contend on one bucket, half spread across many.
			route := "GET /shared"
			if i%2 == 0 {
				route = "GET /route/" + itoa(i)
			}
			for range 50 {
				b := l.bucketFor(route)
				if err := b.wait(ctx); err != nil {
					t.Error(err)
					return
				}
				// Simulate a response refreshing the window.
				h := http.Header{}
				h.Set("X-RateLimit-Limit", "5")
				h.Set("X-RateLimit-Remaining", "4")
				h.Set("X-RateLimit-Reset-After", "0.01")
				b.update(h)
			}
		}(i)
	}
	wg.Wait()
}

// TestStressBucketExhaustion checks that a bucket with nothing left actually
// blocks, and then recovers once the window passes - the behaviour that stops
// a bot from earning a 429.
func TestStressBucketExhaustion(t *testing.T) {
	b := &bucket{}
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "1")
	h.Set("X-RateLimit-Remaining", "0")
	h.Set("X-RateLimit-Reset-After", "0.05")
	b.update(h)

	start := time.Now()
	if err := b.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 40*time.Millisecond {
		t.Errorf("an exhausted bucket returned after %v, expected it to wait ~50ms", waited)
	}
}

// TestStressBucketRespectsContext makes sure a cancelled context escapes a
// rate-limit wait rather than hanging the caller.
func TestStressBucketRespectsContext(t *testing.T) {
	b := &bucket{}
	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "0")
	h.Set("X-RateLimit-Reset-After", "30")
	b.update(h)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := b.wait(ctx); err == nil {
		t.Fatal("expected the wait to be cancelled")
	}
	if waited := time.Since(start); waited > time.Second {
		t.Errorf("cancellation took %v - the wait is not watching the context", waited)
	}
}

// TestStressSnowflakeConcurrent exercises the custom marshalers from many
// goroutines, since they are on the hot path of every payload.
func TestStressSnowflakeConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				b, err := json.Marshal(Snowflake(175928847299117063))
				if err != nil {
					t.Error(err)
					return
				}
				var s Snowflake
				if err := json.Unmarshal(b, &s); err != nil {
					t.Error(err)
					return
				}
				if s != 175928847299117063 {
					t.Errorf("round trip produced %d", s)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestStressMixedFrames pushes every frame shape the parser handles through
// it, including the malformed ones, to be sure none of them panic or corrupt
// the shared scratch buffer.
func TestStressMixedFrames(t *testing.T) {
	c := testClient()
	On(c, func(*MessageCreate) {})
	On(c, func(*Ready) {})

	frames := [][]byte{
		[]byte(messageCreateFrame),
		bigGuildCreate(20, 5, 3),
		[]byte(`{"op":11,"d":null}`),
		[]byte(`{"t":"TYPING_START","s":2,"op":0,"d":{"user_id":"1"}}`),
		[]byte(`{"d":{"content":"out of order"},"op":0,"t":"MESSAGE_CREATE","s":3}`),
		[]byte(`{"t":"READY","s":4,"op":0,"d":{"v":10,"session_id":"x","user":{"id":"1"}}}`),
		[]byte(`{"op":0,"t":"UNKNOWN_EVENT","s":5,"d":{"whatever":true}}`),
	}

	var g gateway
	ctx := context.Background()
	for range 200 {
		for _, f := range frames {
			if err := g.handleFrame(ctx, c, f); err != nil {
				t.Fatalf("frame %s: %v", f, err)
			}
		}
	}
}

// TestStressMalformedFrames feeds the parser broken input. It should return
// errors, never panic - a malformed frame from the network must not take the
// process down.
func TestStressMalformedFrames(t *testing.T) {
	c := testClient()
	On(c, func(*MessageCreate) {})

	bad := []string{
		``,
		`{`,
		`[]`,
		`"a string"`,
		`{"op":`,
		`{"op":0,"t":"MESSAGE_CREATE","d":`,
		`{"op":"not-a-number"}`,
		`{"op":0,"t":123,"d":{}}`,
		`{"op":0,"t":"MESSAGE_CREATE","s":"nope","d":{}}`,
	}

	var g gateway
	ctx := context.Background()
	for _, s := range bad {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panicked on %q: %v", s, r)
				}
			}()
			_ = g.handleFrame(ctx, c, []byte(s)) // an error is fine, a panic is not
		}()
	}
}
