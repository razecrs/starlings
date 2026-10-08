package starlings

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// limiter tracks Discord's rate limits: one bucket per route, shared between
// routes that Discord reports as one bucket, plus the bot's global limit.
type limiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket // by route
	shared  map[string]*bucket // by Discord bucket hash and major parameter

	// globalUntil is set when Discord returns a global 429; every request
	// waits for it.
	globalMu    sync.Mutex
	globalUntil time.Time

	// global paces bot-token requests to Discord's global limit before a 429
	// can happen. Interaction callbacks are exempt.
	global tokenBucket

	invalid invalidCounter
}

func newLimiter() *limiter {
	l := &limiter{buckets: make(map[string]*bucket, 16), shared: make(map[string]*bucket, 16)}
	l.global.setRate(defaultGlobalRate)
	return l
}

// defaultGlobalRate is Discord's documented global limit for bots.
const defaultGlobalRate = 50

func (l *limiter) bucketFor(route string) *bucket {
	l.mu.RLock()
	b := l.buckets[route]
	l.mu.RUnlock()
	if b != nil {
		return b
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if b = l.buckets[route]; b != nil {
		return b
	}
	b = newBucket()
	l.buckets[route] = b
	return b
}

// share points route at the bucket Discord says it belongs to. Routes that
// share a limit (Discord reports the same X-RateLimit-Bucket for them) then
// share one local count, so using one of them correctly delays the others.
// Buckets stay separate per major parameter, as Discord counts them.
func (l *limiter) share(route string, current *bucket, h http.Header) {
	hash := h.Get("X-RateLimit-Bucket")
	if hash == "" {
		return
	}
	key := hash + ":" + majorParameter(route)
	l.mu.Lock()
	defer l.mu.Unlock()
	existing, ok := l.shared[key]
	if !ok {
		l.shared[key] = current
		return
	}
	if existing != current && l.buckets[route] == current {
		l.buckets[route] = existing
	}
}

// majorParameter returns the top-level resource of a route: the channel,
// guild, or webhook (with its token) it acts on.
func majorParameter(route string) string {
	if i := strings.IndexByte(route, ' '); i >= 0 {
		route = route[i+1:]
	}
	parts := strings.Split(strings.Trim(route, "/"), "/")
	for i, part := range parts {
		switch part {
		case "channels", "guilds":
			if i+1 < len(parts) {
				return part + "/" + parts[i+1]
			}
		case "webhooks":
			switch {
			case i+2 < len(parts):
				return part + "/" + parts[i+1] + "/" + parts[i+2]
			case i+1 < len(parts):
				return part + "/" + parts[i+1]
			}
		}
	}
	return ""
}

// waitGlobal blocks while a global rate limit is in force.
func (l *limiter) waitGlobal(ctx context.Context) error {
	l.globalMu.Lock()
	until := l.globalUntil
	l.globalMu.Unlock()
	return sleepContext(ctx, time.Until(until))
}

// handle429 reads the retry delay from a 429 response and, if the limit was
// global, records it so every other request waits too.
func (l *limiter) handle429(resp *http.Response, body []byte) (time.Duration, bool) {
	wait := time.Second
	global := strings.EqualFold(resp.Header.Get("X-RateLimit-Global"), "true")
	var payload struct {
		RetryAfter float64 `json:"retry_after"`
		Global     bool    `json:"global"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		if payload.RetryAfter > 0 {
			wait = time.Duration(payload.RetryAfter * float64(time.Second))
		}
		global = global || payload.Global
	}

	if payload.RetryAfter <= 0 {
		if v := resp.Header.Get("Retry-After"); v != "" {
			if secs, err := strconv.ParseFloat(v, 64); err == nil {
				wait = time.Duration(secs * float64(time.Second))
			}
		}
	}

	if global {
		l.globalMu.Lock()
		if until := time.Now().Add(wait); until.After(l.globalUntil) {
			l.globalUntil = until
		}
		l.globalMu.Unlock()
	}
	return wait, global
}

// bucket is the state of one Discord rate-limit bucket.
//
// Until the first response describes the bucket, only one request is sent;
// the rest wait for it. That turns "fire ten requests at a new route" into one
// request followed by as many as the reported limit allows, instead of ten
// requests and nine 429s.
type bucket struct {
	mu        sync.Mutex
	limit     int
	remaining int // -1 until a response tells us
	resetAt   time.Time
	probing   bool          // a request is learning the limits
	learned   chan struct{} // closed when the probe finishes
	unlimited bool          // the route sends no rate-limit headers

	// refilled is set when the local count was reset to limit because the
	// window ended. Until a response gives the new reset time, responses can
	// only lower the count, and the bucket is not refilled again.
	refilled bool
}

func newBucket() *bucket { return &bucket{remaining: -1} }

// errRateLimitWouldWait is returned under FailFast instead of waiting.
var errRateLimitWouldWait = errors.New("starlings: rate limit would wait")

// wait blocks until the bucket has room, then claims one request from it. It
// returns how long it waited.
//
// The lock is held only for the accounting, not across the HTTP call, so
// several requests in a bucket with headroom still go out concurrently.
func (b *bucket) wait(ctx context.Context) error {
	_, err := b.waitFor(ctx)
	return err
}

func (b *bucket) waitFor(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	failFast := isFailFast(ctx)
	for {
		b.mu.Lock()
		if b.unlimited || b.remaining > 0 {
			if b.remaining > 0 {
				b.remaining--
			}
			b.mu.Unlock()
			return time.Since(start), nil
		}
		if b.remaining < 0 {
			if !b.probing {
				b.probing = true
				b.learned = make(chan struct{})
				b.mu.Unlock()
				return time.Since(start), nil
			}
			learned := b.learned
			b.mu.Unlock()
			if failFast {
				return 0, &RateLimitError{RetryAfter: time.Second}
			}
			select {
			case <-ctx.Done():
				return time.Since(start), ctx.Err()
			case <-learned:
			}
			continue
		}
		d := time.Until(b.resetAt)
		if d <= 0 && !b.refilled {
			// The window has passed. Discord refills the bucket to its limit,
			// so carry on at that rate; the next response gives the new reset.
			b.remaining = -1
			if b.limit > 0 {
				b.remaining = b.limit
				b.refilled = true
			}
			b.mu.Unlock()
			continue
		}
		if b.refilled {
			// Used up the refill before any response arrived: wait for one.
			d = max(d, 10*time.Millisecond)
		}
		b.mu.Unlock()
		if failFast {
			return 0, &RateLimitError{RetryAfter: d}
		}
		if err := sleepContext(ctx, d); err != nil {
			return time.Since(start), err
		}
	}
}

// update refreshes the bucket from a response's rate-limit headers and ends
// any probe.
func (b *bucket) update(h http.Header) {
	remaining := h.Get("X-RateLimit-Remaining")
	resetAfter := h.Get("X-RateLimit-Reset-After")

	b.mu.Lock()
	defer b.mu.Unlock()
	defer b.endProbeLocked()

	if remaining == "" && resetAfter == "" {
		if b.remaining < 0 {
			b.unlimited = true
		}
		return
	}
	b.unlimited = false
	if v, err := strconv.Atoi(h.Get("X-RateLimit-Limit")); err == nil {
		b.limit = v
	}
	reset := b.resetAt
	if v, err := strconv.ParseFloat(resetAfter, 64); err == nil {
		after := time.Duration(v * float64(time.Second))
		reset = time.Now().Add(after + min(resetMargin, after/10))
	}
	if v, err := strconv.Atoi(remaining); err == nil {
		// Responses to concurrent requests arrive in any order. Within one
		// window the count can only go down, so a late response reporting
		// more room than is left must not reopen the bucket.
		sameWindow := b.refilled ||
			(b.remaining >= 0 && !b.resetAt.IsZero() && reset.Sub(b.resetAt).Abs() < 2*resetMargin)
		if sameWindow {
			b.remaining = min(b.remaining, v)
		} else {
			b.remaining = v
		}
	}
	b.resetAt = reset
	b.refilled = false
}

// resetMargin is added to every bucket reset, up to a tenth of the window.
// The reset time is measured from when the response arrived, so without a
// margin a burst sent the moment the local clock says "reset" can reach
// Discord just before its window ends.
const resetMargin = 15 * time.Millisecond

// settle ends a probe whose request got no response, such as a transport
// error, so the next request can try instead.
func (b *bucket) settle() {
	b.mu.Lock()
	b.endProbeLocked()
	b.mu.Unlock()
}

func (b *bucket) endProbeLocked() {
	if b.probing {
		b.probing = false
		close(b.learned)
	}
}

// tokenBucket is a plain token bucket used for the global limit.
type tokenBucket struct {
	mu     sync.Mutex
	rate   float64
	tokens float64
	last   time.Time
}

func (t *tokenBucket) setRate(perSecond int) {
	t.mu.Lock()
	t.rate = float64(perSecond)
	t.tokens = t.rate
	t.last = time.Now()
	t.mu.Unlock()
}

// take waits for one token. A rate of zero or less disables the bucket.
func (t *tokenBucket) take(ctx context.Context) error {
	for {
		t.mu.Lock()
		if t.rate <= 0 {
			t.mu.Unlock()
			return nil
		}
		now := time.Now()
		t.tokens = min(t.rate, t.tokens+now.Sub(t.last).Seconds()*t.rate)
		t.last = now
		if t.tokens >= 1 {
			t.tokens--
			t.mu.Unlock()
			return nil
		}
		wait := time.Duration((1 - t.tokens) / t.rate * float64(time.Second))
		t.mu.Unlock()
		if isFailFast(ctx) {
			return &RateLimitError{RetryAfter: wait, Global: true}
		}
		if err := sleepContext(ctx, wait); err != nil {
			return err
		}
	}
}

// invalidCounter counts the 401, 403, and 429 responses Discord's Cloudflare
// layer counts. Ten thousand in ten minutes gets the bot's IP blocked.
type invalidCounter struct {
	mu     sync.Mutex
	start  time.Time
	count  int
	warned int
}

const (
	invalidWindow = 10 * time.Minute
	invalidLimit  = 10_000
)

// add records one invalid response and returns the count in the current
// window, plus whether this one crossed a warning threshold.
func (c *invalidCounter) add(now time.Time) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.start.IsZero() || now.Sub(c.start) >= invalidWindow {
		c.start, c.count, c.warned = now, 0, 0
	}
	c.count++
	for _, threshold := range []int{invalidLimit / 4, invalidLimit / 2, invalidLimit * 9 / 10} {
		if c.count == threshold && c.warned < threshold {
			c.warned = threshold
			return c.count, true
		}
	}
	return c.count, false
}

func (c *invalidCounter) current(now time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.start.IsZero() || now.Sub(c.start) >= invalidWindow {
		return 0
	}
	return c.count
}

// RateLimitError is returned instead of waiting when a request made with a
// FailFast context would have to wait for a rate limit.
type RateLimitError struct {
	RetryAfter time.Duration
	Global     bool
}

func (e *RateLimitError) Error() string {
	return "starlings: rate limited, retry after " + e.RetryAfter.Round(time.Millisecond).String()
}

type failFastKey struct{}

// FailFast returns a context under which REST calls return a *RateLimitError
// instead of waiting for a rate limit. Use it for work that is pointless
// later, such as renaming a channel inside a three-second interaction
// window. Other calls keep waiting as usual.
func FailFast(ctx context.Context) context.Context {
	return context.WithValue(ctx, failFastKey{}, true)
}

func isFailFast(ctx context.Context) bool {
	v, _ := ctx.Value(failFastKey{}).(bool)
	return v
}

// WithGlobalRateLimit sets how many requests per second the client sends
// before Discord's global limit would apply. The default is Discord's 50.
// Raise it only if Discord granted the bot a higher limit; zero disables
// local pacing and relies on 429 responses alone.
func WithGlobalRateLimit(perSecond int) Option {
	return func(c *Client) { c.rest.limiter.global.setRate(perSecond) }
}

// InvalidRequests reports how many 401, 403, and 429 responses Discord
// counted against the bot in the current ten-minute window. At 10,000,
// Cloudflare blocks the bot's IP address for a while.
func (c *Client) InvalidRequests() int {
	return c.rest.limiter.invalid.current(time.Now())
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
