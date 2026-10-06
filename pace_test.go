package starlings

import (
	"context"
	"testing"
	"time"
)

// TestPacemakerSpacesRequests verifies paced requests queue one at a time:
// three requests with a 50ms gap take at least 100ms in total.
func TestPacemakerSpacesRequests(t *testing.T) {
	var p pacemaker
	p.set(50 * time.Millisecond)

	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := p.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 100*time.Millisecond {
		t.Fatalf("3 paced requests took %v, want >= 100ms", d)
	}
}

// TestPacemakerZeroGapIsInstant verifies the bot default (no pacing) adds
// no measurable delay.
func TestPacemakerZeroGapIsInstant(t *testing.T) {
	var p pacemaker // gap zero

	start := time.Now()
	for i := 0; i < 50; i++ {
		if err := p.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("50 unpaced requests took %v, want ~instant", d)
	}
}

// TestPacemakerHonoursContext verifies a cancelled context releases the wait
// instead of sleeping out the full gap.
func TestPacemakerHonoursContext(t *testing.T) {
	var p pacemaker
	p.set(time.Hour)
	if err := p.wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := p.wait(ctx); err == nil {
		t.Fatal("want context error")
	} else if time.Since(start) > time.Second {
		t.Fatalf("cancelled wait took %v, want ~instant", time.Since(start))
	}
}
