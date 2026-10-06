package starlings

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestOwnReactionRouteSharesLimiterState(t *testing.T) {
	l := newLimiter()
	route := ownReactionRoute(123)
	b := l.bucketFor(route)

	h := http.Header{}
	h.Set("X-RateLimit-Limit", "1")
	h.Set("X-RateLimit-Remaining", "0")
	h.Set("X-RateLimit-Reset-After", "0.05")
	b.update(h)

	if got := l.bucketFor(ownReactionRoute(123)); got != b {
		t.Fatal("add and remove reaction did not resolve to the same local bucket")
	}

	start := time.Now()
	if err := l.bucketFor(ownReactionRoute(123)).wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 40*time.Millisecond {
		t.Fatalf("shared reaction bucket returned after %v; expected it to honor the known reset", waited)
	}

	if got := l.bucketFor(ownReactionRoute(456)); got == b {
		t.Fatal("different channel IDs must remain separate major-parameter buckets")
	}
}
