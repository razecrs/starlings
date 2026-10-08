package starlings

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

var restMessagePayload = []byte(`{"id":"175928847299117063","channel_id":"1220169905119297537","guild_id":"1208479274789634118","content":"hello from a representative REST response","author":{"id":"80351110224678912","username":"starling","discriminator":"0","bot":true},"attachments":[],"embeds":[],"components":[],"mentions":[],"mention_roles":[],"pinned":false,"tts":false,"type":0}`)

func BenchmarkStarlogRecord(b *testing.B) {
	log := NewStarlog("bench", StarlogStreaming(), StarlogStreamArt(false),
		StarlogOutput(io.Discard), StarlogHistory(250))
	logger := log.Logger()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		logger.Info("gateway dispatch", "shard", 0, "event", "MESSAGE_CREATE")
	}
}

func BenchmarkStarlogPetFrameCached(b *testing.B) {
	pet := NewStarPet(StarPetNova)
	_, _ = renderStarPetPicture(pet, StarlogBuild, time.Unix(0, 0), 22)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = renderStarPetPicture(pet, StarlogBuild, time.Unix(0, 0), 22)
	}
}

// bigGuildCreate builds a GUILD_CREATE frame the size a real one arrives at:
// Discord sends one of these for every guild the moment a bot connects, with
// the full member, channel and role list inline. It is the largest thing on
// the wire and the clearest case for not parsing what nobody asked for.
func bigGuildCreate(members, channels, roles int) []byte {
	var b strings.Builder
	b.WriteString(`{"t":"GUILD_CREATE","s":3,"op":0,"d":{"id":"41771983423143937",`)
	b.WriteString(`"name":"Benchmark Guild","owner_id":"80351110224678912","member_count":`)
	fmt.Fprintf(&b, `%d,"large":true,`, members)

	b.WriteString(`"roles":[`)
	for i := range roles {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"%d","name":"role-%d","color":%d,"position":%d,`+
			`"permissions":"104324673","hoist":false,"managed":false,"mentionable":false}`,
			900000000000000000+i, i, i*1000, i)
	}
	b.WriteString(`],"channels":[`)
	for i := range channels {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"%d","type":0,"name":"channel-%d","position":%d,`+
			`"topic":"a topic that is reasonably long, as topics tend to be","nsfw":false,`+
			`"rate_limit_per_user":0,"parent_id":"800000000000000000"}`,
			800000000000000000+i, i, i)
	}
	b.WriteString(`],"members":[`)
	for i := range members {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"user":{"id":"%d","username":"user%d","global_name":"User %d",`+
			`"discriminator":"0","avatar":"a_1234567890abcdef1234567890abcdef","bot":false},`+
			`"nick":null,"roles":["900000000000000000"],"joined_at":"2021-05-19T00:00:00.000000+00:00",`+
			`"deaf":false,"mute":false,"pending":false}`,
			700000000000000000+i, i, i)
	}
	b.WriteString(`]}}`)
	return []byte(b.String())
}

// BenchmarkGuildCreateSkipped is the case that justifies the frame parser:
// a large payload arriving at a bot with no handler for it.
func BenchmarkGuildCreateSkipped(b *testing.B) {
	c := New(WithToken("token"), WithLogger(discardLogger()), WithStateCache(StateConfig{}))
	var g gateway
	frame := bigGuildCreate(500, 40, 25)
	ctx := context.Background()
	b.SetBytes(int64(len(frame)))

	b.ReportAllocs()
	for b.Loop() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGuildCreateDispatched is the same payload fully decoded, for
// comparison. The gap between the two is what a bot saves per unhandled event.
func BenchmarkGuildCreateDispatched(b *testing.B) {
	c := New(WithToken("token"), WithLogger(discardLogger()), WithStateCache(StateConfig{}))
	On(c, func(*GuildCreate) {})
	var g gateway
	frame := bigGuildCreate(500, 40, 25)
	ctx := context.Background()
	b.SetBytes(int64(len(frame)))

	b.ReportAllocs()
	for b.Loop() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGuildCreateWrongHandler is the realistic mixed case: the bot has
// handlers, just not for this event. It must cost the same as the skip case,
// not the dispatch case - if it does not, the lookup is on the wrong side of
// the decode.
func BenchmarkGuildCreateWrongHandler(b *testing.B) {
	c := New(WithToken("token"), WithLogger(discardLogger()), WithStateCache(StateConfig{}))
	On(c, func(*MessageCreate) {})
	On(c, func(*InteractionCreate) {})
	var g gateway
	frame := bigGuildCreate(500, 40, 25)
	ctx := context.Background()
	b.SetBytes(int64(len(frame)))

	b.ReportAllocs()
	for b.Loop() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnowflakeUnmarshal covers the single most common decode in the
// library: Discord sends IDs as strings, and a big payload is mostly IDs.
func BenchmarkSnowflakeUnmarshal(b *testing.B) {
	data := []byte(`"175928847299117063"`)
	b.ReportAllocs()
	for b.Loop() {
		var s Snowflake
		if err := json.Unmarshal(data, &s); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnowflakeMarshal covers the outbound direction.
func BenchmarkSnowflakeMarshal(b *testing.B) {
	const s Snowflake = 175928847299117063
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(s); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSendDataMarshal covers the REST write path: encoding an outgoing
// message body.
func BenchmarkSendDataMarshal(b *testing.B) {
	data := SendData{
		Content: "hello there, this is a message of a fairly typical length",
		Embeds: []Embed{{
			Title:       "An embed",
			Description: "With a description of the sort a bot actually sends",
			Color:       0x5865F2,
			Fields: []EmbedField{
				{Name: "one", Value: "first value", Inline: true},
				{Name: "two", Value: "second value", Inline: true},
			},
		}},
		AllowedMentions: NoMentions(),
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRESTJSONDecodeStream(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var message Message
		if err := decodeLimitedJSON(bytes.NewReader(restMessagePayload), maxRESTResponse, &message); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRESTJSONDecodeBuffered(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		data, err := readLimitedBody(bytes.NewReader(restMessagePayload), maxRESTResponse)
		if err != nil {
			b.Fatal(err)
		}
		var message Message
		if err := json.Unmarshal(data, &message); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDispatchManyHandlers checks that adding handlers for an event stays
// cheap - the payload is decoded once, then handed to each.
func BenchmarkDispatchManyHandlers(b *testing.B) {
	for _, n := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("handlers=%d", n), func(b *testing.B) {
			c := testClient()
			for range n {
				On(c, func(*MessageCreate) {})
			}
			var g gateway
			frame := []byte(messageCreateFrame)
			ctx := context.Background()

			b.ReportAllocs()
			for b.Loop() {
				if err := g.handleFrame(ctx, c, frame); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSlotLookup isolates the map lookup that gates every frame.
func BenchmarkSlotLookup(b *testing.B) {
	c := testClient()
	On(c, func(*MessageCreate) {})
	b.ReportAllocs()
	for b.Loop() {
		if c.slotFor("STARLINGS_BENCHMARK_UNKNOWN_EVENT") != nil {
			b.Fatal("unexpected slot")
		}
	}
}
