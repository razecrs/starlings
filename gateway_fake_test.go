package starlings

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeGateway is a minimal Discord gateway for tests. Each connection gets
// HELLO, then on is called with every frame the client sends, so a test can
// script READY, closes, and other replies.
type fakeGateway struct {
	t      *testing.T
	server *httptest.Server

	mu     sync.Mutex
	frames []fakeFrame
	on     func(conn *websocket.Conn, frame fakeFrame)
}

type fakeFrame struct {
	Op Opcode         `json:"op"`
	D  jsontext.Value `json:"d"`
}

func newFakeGateway(t *testing.T, on func(conn *websocket.Conn, frame fakeFrame)) *fakeGateway {
	t.Helper()
	g := &fakeGateway{t: t, on: on}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"op":10,"d":{"heartbeat_interval":45000}}`))
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var frame fakeFrame
			if json.Unmarshal(data, &frame) != nil {
				continue
			}
			g.mu.Lock()
			g.frames = append(g.frames, frame)
			g.mu.Unlock()
			if g.on != nil {
				g.on(conn, frame)
			}
		}
	}))
	t.Cleanup(g.server.Close)
	return g
}

// client returns an uncompressed single-shard client pointed at the fake.
func (g *fakeGateway) client(opts ...Option) *Client {
	base := []Option{WithShard(0, 1), WithGatewayCompression(false), WithLogger(discardLogger())}
	c := New("Bot token", append(base, opts...)...)
	c.gatewayBase = "ws" + strings.TrimPrefix(g.server.URL, "http")
	return c
}

func (g *fakeGateway) sent(op Opcode) []fakeFrame {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []fakeFrame
	for _, f := range g.frames {
		if f.Op == op {
			out = append(out, f)
		}
	}
	return out
}

func writeDispatch(conn *websocket.Conn, seq int, name, data string) {
	frame := `{"op":0,"s":` + itoa(seq) + `,"t":"` + name + `","d":` + data + `}`
	_ = conn.Write(context.Background(), websocket.MessageText, []byte(frame))
}

const fakeReady = `{"v":10,"user":{"id":"1","username":"bot"},"session_id":"s","resume_gateway_url":"","guilds":[],"application":{"id":"2"}}`

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestOnlineFollowsConnectionNotFirstReady(t *testing.T) {
	var mu sync.Mutex
	var current *websocket.Conn
	g := newFakeGateway(t, func(conn *websocket.Conn, f fakeFrame) {
		if f.Op == OpIdentify {
			mu.Lock()
			current = conn
			mu.Unlock()
			writeDispatch(conn, 1, "READY", fakeReady)
		}
	})
	c := g.client()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.RunContext(ctx)

	waitFor(t, "first READY", c.Online)
	disconnected := make(chan struct{}, 1)
	c.On(func(*Disconnect) { disconnected <- struct{}{} })

	mu.Lock()
	_ = current.Close(websocket.StatusCode(CloseSessionTimedOut), "test")
	mu.Unlock()
	<-disconnected
	if c.Online() {
		t.Fatal("Online stayed true after the connection dropped")
	}
	waitFor(t, "READY after reconnect", c.Online)
}

func TestCloseDuringIdentifyReturnsPromptly(t *testing.T) {
	identified := make(chan struct{}, 1)
	g := newFakeGateway(t, func(_ *websocket.Conn, f fakeFrame) {
		if f.Op == OpIdentify {
			identified <- struct{}{} // never answer with READY
		}
	})
	c := g.client()
	done := make(chan error, 1)
	go func() { done <- c.RunContext(context.Background()) }()
	<-identified
	_ = c.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunContext after Close = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunContext did not return after Close during IDENTIFY")
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(g.sent(OpIdentify)); n != 1 {
		t.Fatalf("client identified %d times; it reconnected after Close", n)
	}
}

func TestDisconnectReportsCloseCode(t *testing.T) {
	g := newFakeGateway(t, func(conn *websocket.Conn, f fakeFrame) {
		if f.Op == OpIdentify {
			_ = conn.Close(websocket.StatusCode(CloseDecodeError), "bad payload")
		}
	})
	c := g.client()
	got := make(chan *Disconnect, 1)
	c.On(func(d *Disconnect) {
		select {
		case got <- d:
		default:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.RunContext(ctx)

	d := <-got
	if d.CloseCode != CloseDecodeError || d.CloseReason != "bad payload" {
		t.Fatalf("Disconnect = code %d reason %q, want 4002 \"bad payload\"", d.CloseCode, d.CloseReason)
	}
	err := (&GatewayCloseError{Code: d.CloseCode}).Error()
	if !strings.Contains(err, "payload this bot sent") {
		t.Fatalf("4002 message %q does not explain the cause", err)
	}
}

func TestMemberChunkingRequestsIncompleteGuilds(t *testing.T) {
	g := newFakeGateway(t, func(conn *websocket.Conn, f fakeFrame) {
		if f.Op == OpIdentify {
			writeDispatch(conn, 1, "READY", fakeReady)
			writeDispatch(conn, 2, "GUILD_CREATE", `{"id":"100","name":"big","member_count":500,"members":[]}`)
			writeDispatch(conn, 3, "GUILD_CREATE", `{"id":"200","name":"small","member_count":1,"members":[{"user":{"id":"9","username":"x"}}]}`)
		}
	})
	c := g.client(WithIntents(IntentGuilds|IntentGuildMembers), WithMemberChunking(true))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.RunContext(ctx)

	waitFor(t, "member request", func() bool { return len(g.sent(OpRequestGuildMembers)) > 0 })
	time.Sleep(50 * time.Millisecond)
	requests := g.sent(OpRequestGuildMembers)
	if len(requests) != 1 {
		t.Fatalf("sent %d member requests, want 1 (only the incomplete guild)", len(requests))
	}
	var body struct {
		GuildID Snowflake `json:"guild_id"`
		Query   *string   `json:"query"`
		Limit   *int      `json:"limit"`
	}
	if err := json.Unmarshal(requests[0].D, &body); err != nil {
		t.Fatal(err)
	}
	if body.GuildID != 100 || body.Query == nil || *body.Query != "" || body.Limit == nil || *body.Limit != 0 {
		t.Fatalf("member request = %s, want guild 100 with query \"\" and limit 0", requests[0].D)
	}
}

func TestSendWindowKeepsReserveForPriority(t *testing.T) {
	var w sendWindow
	w.reset(time.Now())
	ctx := context.Background()
	for range gatewaySendLimit - gatewaySendReserved {
		if err := w.take(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := w.take(short, false); err == nil {
		t.Fatal("an ordinary send used a reserved slot")
	}
	for range gatewaySendReserved {
		if err := w.take(ctx, true); err != nil {
			t.Fatalf("priority send was refused: %v", err)
		}
	}
	short2, cancel2 := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel2()
	if err := w.take(short2, true); err == nil {
		t.Fatal("window allowed more than 120 sends")
	}
}
