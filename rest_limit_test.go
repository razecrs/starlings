package starlings

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// discordBuckets is an in-process REST server that enforces Discord-style
// buckets: limit requests per window per route key, answering 429 when a
// client sends more than the headers allowed.
type discordBuckets struct {
	limit  int
	window time.Duration
	hash   func(r *http.Request) string

	mu       sync.Mutex
	counts   map[string]int
	resets   map[string]time.Time
	accepted atomic.Int32
	rejected atomic.Int32
}

func (d *discordBuckets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.Path
	if d.hash != nil {
		key = d.hash(r)
	}
	d.mu.Lock()
	now := time.Now()
	if d.resets[key].Before(now) {
		d.resets[key] = now.Add(d.window)
		d.counts[key] = 0
	}
	d.counts[key]++
	used, reset := d.counts[key], d.resets[key]
	d.mu.Unlock()

	after := strconv.FormatFloat(time.Until(reset).Seconds(), 'f', 3, 64)
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(d.limit))
	w.Header().Set("X-RateLimit-Reset-After", after)
	w.Header().Set("X-RateLimit-Bucket", key)
	if used > d.limit {
		d.rejected.Add(1)
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"You are being rate limited.","retry_after":` + after + `,"global":false}`))
		return
	}
	d.accepted.Add(1)
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(d.limit-used))
	w.WriteHeader(http.StatusNoContent)
}

func newDiscordBuckets(limit int, window time.Duration) *discordBuckets {
	return &discordBuckets{limit: limit, window: window, counts: map[string]int{}, resets: map[string]time.Time{}}
}

func TestNewBucketLearnsLimitBeforeBursting(t *testing.T) {
	server := newDiscordBuckets(5, 300*time.Millisecond)
	c := clientServedBy(server)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			if err := c.Typing(context.Background(), 42); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := server.rejected.Load(); n != 0 {
		t.Fatalf("server answered %d requests with 429; the client should learn the limit first", n)
	}
	if n := server.accepted.Load(); n != 12 {
		t.Fatalf("accepted %d requests, want 12", n)
	}
}

func TestRoutesInOneDiscordBucketShareLimits(t *testing.T) {
	server := newDiscordBuckets(1, 300*time.Millisecond)
	// Discord reports one bucket for both routes in the same channel.
	server.hash = func(r *http.Request) string { return "shared-" + r.URL.Path[:len("/api/v10/channels/42")] }
	c := clientServedBy(server)
	ctx := context.Background()
	// A route's bucket is only known after its first response, so the first
	// delete can earn one 429. From then on both routes share one count.
	for range 3 {
		if err := c.Typing(ctx, 42); err != nil {
			t.Fatal(err)
		}
		if err := c.DeleteMessage(ctx, 42, 7, ""); err != nil {
			t.Fatal(err)
		}
	}
	if n := server.rejected.Load(); n > 1 {
		t.Fatalf("server answered %d requests with 429; after learning the shared bucket there should be none", n)
	}
}

func TestFailFastReturnsRateLimitError(t *testing.T) {
	server := newDiscordBuckets(1, 10*time.Second)
	c := clientServedBy(server)
	if err := c.Typing(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	err := c.Typing(FailFast(context.Background()), 42)
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter <= 0 {
		t.Fatalf("FailFast call = %v, want *RateLimitError", err)
	}
	if server.rejected.Load() != 0 {
		t.Fatal("FailFast still sent the request")
	}
}

func TestInteractionCallbacksSkipPacing(t *testing.T) {
	c := clientServedBy(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), WithPacing(time.Second))
	ctx := context.Background()
	if err := c.Typing(ctx, 42); err != nil {
		t.Fatal(err)
	}
	i := &InteractionCreate{Interaction: Interaction{ID: 1, Token: "t"}}
	i.c = c
	start := time.Now()
	if err := i.ReplyContext(ctx, "hi"); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited > 500*time.Millisecond {
		t.Fatalf("interaction callback waited %v behind the pacer", waited)
	}
}

func TestRejectedTokenStopsRequests(t *testing.T) {
	var calls atomic.Int32
	c := clientServedBy(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401: Unauthorized","code":0}`))
	}))
	ctx := context.Background()
	_ = c.Typing(ctx, 42)
	if err := c.Typing(ctx, 43); !errors.Is(err, ErrTokenRejected) {
		t.Fatalf("request after 401 = %v, want ErrTokenRejected", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("server saw %d requests, want 1", calls.Load())
	}
	if c.InvalidRequests() != 1 {
		t.Fatalf("InvalidRequests = %d, want 1", c.InvalidRequests())
	}
}

func TestGlobalRateLimitPacesBotRequests(t *testing.T) {
	c := clientServedBy(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), WithGlobalRateLimit(10))
	start := time.Now()
	for n := range 15 {
		if err := c.Typing(context.Background(), Snowflake(100+n)); err != nil {
			t.Fatal(err)
		}
	}
	// Ten go out at once, the other five at ten per second.
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Fatalf("15 requests at 10/s took %v; the global limit was not applied", elapsed)
	}
}

func TestMajorParameter(t *testing.T) {
	cases := map[string]string{
		"POST /channels/42/messages":             "channels/42",
		"PUT /guilds/7/bans/9":                   "guilds/7",
		"POST /webhooks/5/{token}":               "webhooks/5/{token}",
		"GET /webhooks/5":                        "webhooks/5",
		"POST /interactions/1/callback":          "",
		"REACTION /channels/3/messages/{id}/@me": "channels/3",
		"GET /users/@me":                         "",
	}
	for route, want := range cases {
		if got := majorParameter(route); got != want {
			t.Errorf("majorParameter(%q) = %q, want %q", route, got, want)
		}
	}
}
