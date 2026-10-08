package starlings

import (
	"context"
	"testing"
)

func testClient() *Client {
	c := New(WithToken("token"), WithLogger(discardLogger()))
	return c
}

const messageCreateFrame = `{"t":"MESSAGE_CREATE","s":7,"op":0,"d":{` +
	`"id":"1234567890123456789","channel_id":"987654321098765432",` +
	`"guild_id":"111111111111111111","content":"hello there",` +
	`"author":{"id":"222222222222222222","username":"tester","global_name":"Starlings Tester","bot":false}}}`

func TestHandleFrameDispatches(t *testing.T) {
	c := testClient()

	var got *MessageCreate
	On(c, func(m *MessageCreate) { got = m })

	var g gateway
	if err := g.handleFrame(context.Background(), c, []byte(messageCreateFrame)); err != nil {
		t.Fatalf("handleFrame: %v", err)
	}

	if got == nil {
		t.Fatal("handler was not called")
	}
	if got.Content != "hello there" {
		t.Errorf("Content = %q, want %q", got.Content, "hello there")
	}
	if got.ChannelID != 987654321098765432 {
		t.Errorf("ChannelID = %d", got.ChannelID)
	}
	if got.Author == nil || got.Author.Tag() != "Starlings Tester" {
		t.Errorf("Author = %+v", got.Author)
	}
	if got.Client() != c {
		t.Error("event was not bound to the client")
	}
	if seq := c.seq.Load(); seq != 7 {
		t.Errorf("seq = %d, want 7", seq)
	}
}

func TestHandleFrameSkipsUnhandledEvents(t *testing.T) {
	c := testClient()
	// No handler registered at all: the payload must be skipped without error.
	var g gateway
	if err := g.handleFrame(context.Background(), c, []byte(messageCreateFrame)); err != nil {
		t.Fatalf("handleFrame: %v", err)
	}
	if seq := c.seq.Load(); seq != 7 {
		t.Errorf("seq = %d, want 7 - sequence must be tracked even when the event is skipped", seq)
	}
}

func TestHandleFrameSequenceNeverGoesBackwards(t *testing.T) {
	c := testClient()
	c.seq.Store(100)

	var g gateway
	if err := g.handleFrame(context.Background(), c, []byte(messageCreateFrame)); err != nil {
		t.Fatal(err)
	}
	if seq := c.seq.Load(); seq != 100 {
		t.Errorf("seq = %d, want 100 - a replayed frame must not rewind it", seq)
	}
}

func TestHandleFrameHeartbeatACK(t *testing.T) {
	c := testClient()
	var g gateway
	g.ackPending.Store(true)

	if err := g.handleFrame(context.Background(), c, []byte(`{"op":11,"d":null}`)); err != nil {
		t.Fatal(err)
	}
	if g.ackPending.Load() {
		t.Error("ackPending should be cleared by a heartbeat ACK")
	}
}

func TestHandleFramePayloadBeforeOpcode(t *testing.T) {
	// Discord puts "op" before "d", but the parser must not depend on it.
	frame := `{"d":{"id":"1","channel_id":"2","content":"out of order"},"op":0,"t":"MESSAGE_CREATE","s":1}`

	c := testClient()
	var got *MessageCreate
	On(c, func(m *MessageCreate) { got = m })

	var g gateway
	if err := g.handleFrame(context.Background(), c, []byte(frame)); err != nil {
		t.Fatalf("handleFrame: %v", err)
	}
	if got == nil {
		t.Fatal("handler was not called")
	}
	if got.Content != "out of order" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestReadyUpdatesSessionState(t *testing.T) {
	c := testClient()
	frame := `{"t":"READY","s":1,"op":0,"d":{"v":10,` +
		`"user":{"id":"42","username":"kitty","bot":true},` +
		`"session_id":"abc123","resume_gateway_url":"wss://resume.example",` +
		`"application":{"id":"99"}}}`

	var g gateway
	if err := g.handleFrame(context.Background(), c, []byte(frame)); err != nil {
		t.Fatal(err)
	}

	if sid := c.sessionID.Load(); sid == nil || *sid != "abc123" {
		t.Errorf("sessionID = %v", sid)
	}
	if u := c.resumeURL.Load(); u == nil || *u != "wss://resume.example" {
		t.Errorf("resumeURL = %v", u)
	}
	if self := c.Self(); self == nil || self.Username != "kitty" {
		t.Errorf("Self() = %+v", c.Self())
	}
	if c.ApplicationID() != 99 {
		t.Errorf("ApplicationID() = %d, want 99", c.ApplicationID())
	}
	if err := c.WaitReady(context.Background()); err != nil {
		t.Errorf("WaitReady: %v", err)
	}
}

func TestNormalizeToken(t *testing.T) {
	const raw = "MTIzNDU2Nzg5.abcdef.ghijkl"
	cases := map[string]string{
		raw:           "Bot " + raw, // bare token
		"Bot " + raw:  "Bot " + raw, // already correct
		"Bot" + raw:   "Bot " + raw, // the classic missing-space bug
		"  " + raw:    "Bot " + raw, // stray whitespace
		"Bot  " + raw: "Bot " + raw,
	}
	for in, want := range cases {
		if got := normalizeToken(in); got != want {
			t.Errorf("normalizeToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMultipleHandlersRunInOrder(t *testing.T) {
	c := testClient()
	var order []int
	On(c, func(*MessageCreate) { order = append(order, 1) })
	On(c, func(*MessageCreate) { order = append(order, 2) })
	On(c, func(*MessageCreate) { order = append(order, 3) })

	var g gateway
	if err := g.handleFrame(context.Background(), c, []byte(messageCreateFrame)); err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("handlers ran as %v, want [1 2 3]", order)
	}
}

func TestClientOnPanicsOnBadHandler(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("On should panic when given a non-handler")
		}
	}()
	testClient().On("not a function")
}

// BenchmarkHandleFrameSkipped measures the path a bot spends most of its time
// on: an event it has no handler for.
func BenchmarkHandleFrameSkipped(b *testing.B) {
	c := testClient()
	var g gateway
	frame := []byte(messageCreateFrame)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHandleFrameDispatched measures a fully decoded and delivered event.
func BenchmarkHandleFrameDispatched(b *testing.B) {
	c := testClient()
	On(c, func(*MessageCreate) {})
	var g gateway
	frame := []byte(messageCreateFrame)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if err := g.handleFrame(ctx, c, frame); err != nil {
			b.Fatal(err)
		}
	}
}
