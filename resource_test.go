//go:build comparison

package starlings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// resourceUsage is what one measured workload cost.
type resourceUsage struct {
	name       string
	ops        int
	wall       time.Duration
	totalAlloc uint64 // bytes allocated over the whole run
	peakHeap   uint64 // largest live heap observed while running
	numGC      uint32
	gcPause    time.Duration
}

func (r resourceUsage) perOp() string {
	return fmt.Sprintf("%.2f µs", float64(r.wall.Nanoseconds())/float64(r.ops)/1000)
}

// measure runs fn a fixed number of times and reports what it cost.
//
// Peak heap is sampled rather than exact: ReadMemStats stops the world, so
// polling too often would distort the very thing being measured. At 2 ms the
// perturbation is small and the peak is still representative.
func measure(name string, ops int, fn func()) resourceUsage {
	runtime.GC()
	runtime.GC() // twice, so finalisers from any previous run are settled

	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	var peak uint64
	done := make(chan struct{})
	sampled := make(chan uint64, 1)
	go func() {
		var ms runtime.MemStats
		t := time.NewTicker(2 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				sampled <- peak
				return
			case <-t.C:
				runtime.ReadMemStats(&ms)
				if ms.HeapAlloc > peak {
					peak = ms.HeapAlloc
				}
			}
		}
	}()

	start := time.Now()
	for range ops {
		fn()
	}
	wall := time.Since(start)

	close(done)
	peak = <-sampled

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	return resourceUsage{
		name:       name,
		ops:        ops,
		wall:       wall,
		totalAlloc: after.TotalAlloc - before.TotalAlloc,
		peakHeap:   peak,
		numGC:      after.NumGC - before.NumGC,
		gcPause:    time.Duration(after.PauseTotalNs - before.PauseTotalNs),
	}
}

func mib(b uint64) string { return fmt.Sprintf("%.2f MiB", float64(b)/(1<<20)) }

func report(t *testing.T, title string, rs ...resourceUsage) {
	t.Helper()
	t.Logf("\n=== %s ===", title)
	t.Logf("%-26s %10s %12s %12s %12s %7s %10s",
		"library", "per op", "wall", "total alloc", "peak heap", "GCs", "GC pause")
	for _, r := range rs {
		t.Logf("%-26s %10s %12s %12s %12s %7d %10s",
			r.name, r.perOp(), r.wall.Round(time.Millisecond),
			mib(r.totalAlloc), mib(r.peakHeap), r.numGC, r.gcPause.Round(time.Microsecond))
	}
}

// TestResourceGuildUnhandled measures the case a busy bot lives in: a large
// event arriving that nothing is registered for.
func TestResourceGuildUnhandled(t *testing.T) {
	const ops = 2000
	frame := bigGuildCreate(500, 40, 25)
	t.Logf("frame size: %s", mib(uint64(len(frame))))

	c := testClient()
	On(c, func(*MessageCreate) {}) // handlers, just not for this event
	var g gateway
	ctx := context.Background()

	st := measure("starlings", ops, func() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			t.Fatal(err)
		}
	})

	dg := measure("discordgo", ops, func() {
		var e *discordgo.Event
		if err := json.NewDecoder(bytes.NewBuffer(frame)).Decode(&e); err != nil {
			t.Fatal(err)
		}
		i := &discordgo.GuildCreate{}
		if err := json.Unmarshal(e.RawData, i); err != nil {
			t.Fatal(err)
		}
	})

	report(t, "GUILD_CREATE x2000, no handler for it", st, dg)
}

// TestResourceGuildHandled measures the same event actually being decoded and
// delivered, which is the fair comparison of raw decode cost.
func TestResourceGuildHandled(t *testing.T) {
	const ops = 2000
	frame := bigGuildCreate(500, 40, 25)

	c := testClient()
	On(c, func(*GuildCreate) {})
	var g gateway
	ctx := context.Background()

	st := measure("starlings", ops, func() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			t.Fatal(err)
		}
	})

	dg := measure("discordgo", ops, func() {
		var e *discordgo.Event
		if err := json.NewDecoder(bytes.NewBuffer(frame)).Decode(&e); err != nil {
			t.Fatal(err)
		}
		i := &discordgo.GuildCreate{}
		if err := json.Unmarshal(e.RawData, i); err != nil {
			t.Fatal(err)
		}
	})

	report(t, "GUILD_CREATE x2000, decoded and delivered", st, dg)
}

// TestResourceMessageFlood models a busy guild: a steady stream of small
// messages, most of which the bot does not care about.
func TestResourceMessageFlood(t *testing.T) {
	const ops = 200_000
	frame := []byte(messageCreateFrame)

	c := testClient()
	On(c, func(*MessageCreate) {})
	var g gateway
	ctx := context.Background()

	st := measure("starlings (handled)", ops, func() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			t.Fatal(err)
		}
	})

	idle := testClient()
	var g2 gateway
	stIdle := measure("starlings (unhandled)", ops, func() {
		if err := g2.handleFrame(ctx, idle, frame); err != nil {
			t.Fatal(err)
		}
	})

	dg := measure("discordgo", ops, func() {
		var e *discordgo.Event
		if err := json.NewDecoder(bytes.NewBuffer(frame)).Decode(&e); err != nil {
			t.Fatal(err)
		}
		i := &discordgo.MessageCreate{}
		if err := json.Unmarshal(e.RawData, i); err != nil {
			t.Fatal(err)
		}
	})

	report(t, "MESSAGE_CREATE x200,000", st, stIdle, dg)
}
