//go:build comparison

// Head-to-head benchmarks against discordgo, behind a build tag so discordgo
// is not a permanent dependency of this module.
//
//	go get github.com/bwmarrin/discordgo
//	go test -tags comparison -bench . -benchmem -run XXX
//	go mod tidy   # afterwards, to drop it again
//
// The discordgo path here mirrors Session.onEvent in wsapi.go: decode the
// frame into an Event, then unmarshal RawData into the registered struct.
// Notably discordgo does that second step for every *known* event type
// whether or not a handler is registered (wsapi.go:673-688), which is the
// design difference these benchmarks are measuring.
package starlings

import (
	"bytes"
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// discordgoDispatch replicates discordgo's decode path for one frame.
func discordgoDispatch(b *testing.B, frame []byte, target func() any, handled bool) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(frame)))

	for b.Loop() {
		var e *discordgo.Event
		if err := json.NewDecoder(bytes.NewBuffer(frame)).Decode(&e); err != nil {
			b.Fatal(err)
		}
		if e.Operation != 0 {
			continue
		}
		// discordgo unmarshals the typed struct regardless of handlers.
		i := target()
		if err := json.Unmarshal(e.RawData, i); err != nil {
			b.Fatal(err)
		}
		if handled {
			_ = i
		}
	}
}

func starlingsDispatch(b *testing.B, c *Client, frame []byte) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(frame)))

	var g gateway
	ctx := context.Background()
	for b.Loop() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			b.Fatal(err)
		}
	}
}

// Small MESSAGE_CREATE frame

func BenchmarkCmpMessageHandled_starlings(b *testing.B) {
	c := testClient()
	On(c, func(*MessageCreate) {})
	starlingsDispatch(b, c, []byte(messageCreateFrame))
}

func BenchmarkCmpMessageHandled_discordgo(b *testing.B) {
	discordgoDispatch(b, []byte(messageCreateFrame),
		func() any { return &discordgo.MessageCreate{} }, true)
}

// The unhandled case is the one bots actually spend their time in.
func BenchmarkCmpMessageUnhandled_starlings(b *testing.B) {
	c := testClient() // no handlers at all
	starlingsDispatch(b, c, []byte(messageCreateFrame))
}

func BenchmarkCmpMessageUnhandled_discordgo(b *testing.B) {
	discordgoDispatch(b, []byte(messageCreateFrame),
		func() any { return &discordgo.MessageCreate{} }, false)
}

// Large GUILD_CREATE, 500 members

func BenchmarkCmpGuildHandled_starlings(b *testing.B) {
	c := testClient()
	On(c, func(*GuildCreate) {})
	starlingsDispatch(b, c, bigGuildCreate(500, 40, 25))
}

func BenchmarkCmpGuildHandled_discordgo(b *testing.B) {
	discordgoDispatch(b, bigGuildCreate(500, 40, 25),
		func() any { return &discordgo.GuildCreate{} }, true)
}

func BenchmarkCmpGuildUnhandled_starlings(b *testing.B) {
	c := testClient()
	On(c, func(*MessageCreate) {}) // handlers, just not for this event
	starlingsDispatch(b, c, bigGuildCreate(500, 40, 25))
}

func BenchmarkCmpGuildUnhandled_discordgo(b *testing.B) {
	discordgoDispatch(b, bigGuildCreate(500, 40, 25),
		func() any { return &discordgo.GuildCreate{} }, false)
}

// ID decoding

func BenchmarkCmpSnowflake_starlings(b *testing.B) {
	data := []byte(`"175928847299117063"`)
	b.ReportAllocs()
	for b.Loop() {
		var s Snowflake
		if err := jsonv2.Unmarshal(data, &s); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCmpSnowflake_discordgo(b *testing.B) {
	// discordgo keeps IDs as plain strings, so this is its equivalent cost.
	data := []byte(`"175928847299117063"`)
	b.ReportAllocs()
	for b.Loop() {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			b.Fatal(err)
		}
	}
}
