package starlings

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// readLimit caps a single gateway frame. The default in the websocket package
// is 32 KiB, which a GUILD_CREATE for a large guild blows straight through, so
// it is raised well past anything Discord sends.
const readLimit = 64 << 20

// FatalError wraps a gateway failure that retrying cannot fix: a rejected
// token, or intents the bot has not been granted. RunContext returns one of
// these instead of reconnecting forever.
type FatalError struct {
	Code   CloseCode
	Reason string
}

func (e *FatalError) Error() string {
	s := "starlings: gateway closed permanently: " + e.Code.String()
	if e.Reason != "" {
		s += " (" + sanitizeUntrustedText(e.Reason) + ")"
	}
	return s
}

// GatewayCloseError reports that Discord closed a gateway connection. The
// client reconnects on its own, and the Disconnect event carries the code and
// reason. A fatal code is returned from RunContext as a *FatalError instead.
type GatewayCloseError struct {
	Code   CloseCode
	Reason string
}

func (e *GatewayCloseError) Error() string {
	s := "starlings: gateway closed: " + e.Code.String()
	if e.Reason != "" {
		s += " (" + sanitizeUntrustedText(e.Reason) + ")"
	}
	return s
}

// gateway owns one websocket connection and the state needed to resume it.
type gateway struct {
	conn     *websocket.Conn
	writeMu  sync.Mutex
	inflater *gatewayInflater

	// ackPending is set when a heartbeat goes out and cleared when Discord
	// acknowledges it. A beat falling due while it is still set means the
	// connection is a zombie.
	ackPending atomic.Bool
	connected  atomic.Bool
	live       atomic.Bool // READY or RESUMED received on this connection
	sends      sendWindow
	beatSent   atomic.Int64
	latency    atomic.Int64

	// rdr is reused across frames so that wrapping each one costs no
	// allocation. handleFrame only ever runs on the read loop, so a single
	// reader per connection is safe.
	rdr bytes.Reader

	// scratch buffers the "d" payload on the rare frames where Discord sends
	// it before the fields that say what it is.
	scratch []byte
}

// run keeps a gateway connection alive until ctx is cancelled or a fatal error
// makes reconnecting pointless.
func (g *gateway) run(ctx context.Context, c *Client) error {
	var attempt, repeats int
	var lastClose CloseCode
	for {
		err := g.connect(ctx, c)

		if ctx.Err() != nil {
			return nil
		}

		var fatal *FatalError
		if errors.As(err, &fatal) {
			return err
		}

		// A close caused by something the bot sends repeats after every
		// reconnect. Say so plainly instead of logging the same warning forever.
		var closed *GatewayCloseError
		if errors.As(err, &closed) && closed.Code == lastClose {
			repeats++
		} else {
			repeats = 0
			lastClose = 0
			if closed != nil {
				lastClose = closed.Code
			}
		}
		if repeats == 2 && (lastClose == CloseDecodeError || lastClose == CloseUnknownOpcode || lastClose == CloseRateLimited) {
			c.gatewayLogger().Error("starlings: Discord keeps closing the connection because of something this bot sends; "+
				"check gateway commands made from Ready or GuildCreate handlers", "code", int(lastClose), "reason", lastClose.String())
		}

		attempt++
		delay := backoff(attempt)
		c.gatewayLogger().Warn("starlings: gateway disconnected, reconnecting",
			"err", err, "attempt", attempt, "in", delay)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

// backoff grows from one second to a minute, with jitter so that a fleet of
// bots knocked offline together does not reconnect in lockstep.
func backoff(attempt int) time.Duration {
	const maxDelay = time.Minute
	d := time.Second << min(attempt-1, 6)
	if d > maxDelay {
		d = maxDelay
	}
	return d/2 + time.Duration(rand.Int64N(int64(d/2)))
}

// connect runs one connection from dial to disconnect. It returns the error
// that ended it; the caller decides whether to try again.
func (g *gateway) connect(ctx context.Context, c *Client) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	url, resuming := g.gatewayURL(c)
	if url == "" {
		url = c.gatewayBase
		if url == "" {
			var err error
			// Auto-sharding already populated gatewayBase from /gateway/bot.
			// A manual or user-token connection only needs the public URL, so
			// avoid a heavier authenticated shard-information request here.
			url, err = c.Gateway(ctx)
			if err != nil {
				return fmt.Errorf("looking up gateway URL: %w", err)
			}
		}
		resuming = false
	}

	query := "/?v=" + Version + "&encoding=json"
	if c.compress {
		query += "&compress=zlib-stream"
	}
	conn, _, err := websocket.Dial(ctx, url+query, &websocket.DialOptions{
		HTTPClient: c.rest.http,
	})
	if err != nil {
		return fmt.Errorf("dialing gateway: %w", err)
	}
	conn.SetReadLimit(readLimit)
	g.sends.reset(time.Now())
	g.writeMu.Lock()
	g.conn = conn
	g.writeMu.Unlock()
	if c.compress {
		g.inflater = newGatewayInflater()
	}
	defer func() {
		if g.inflater != nil {
			g.inflater.Close()
			g.inflater = nil
		}
		g.writeMu.Lock()
		if g.conn == conn {
			g.conn = nil
		}
		g.writeMu.Unlock()
		conn.CloseNow()
	}()

	// Discord opens every connection with HELLO, carrying the heartbeat
	// interval. Nothing else may be sent until it arrives.
	interval, err := g.readHello(ctx)
	if err != nil {
		return err
	}

	if resuming {
		c.gatewayLogger().Info("starlings: resuming session")
		err = g.send(ctx, OpResume, resumePayload{
			Token:     c.token,
			SessionID: *c.sessionID.Load(),
			Seq:       c.seq.Load(),
		})
	} else {
		c.gatewayLogger().Info("starlings: identifying", "intents", c.intents)
		err = g.send(ctx, OpIdentify, c.identifyPayload())
	}
	if err != nil {
		return fmt.Errorf("sending identify/resume: %w", err)
	}
	c.emitSynthetic(&Connect{ShardID: c.shard[0]})
	g.connected.Store(true)
	defer func() {
		g.connected.Store(false)
		g.live.Store(false)
		event := &Disconnect{ShardID: c.shard[0]}
		var closed *GatewayCloseError
		var fatal *FatalError
		switch {
		case errors.As(err, &closed):
			event.CloseCode, event.CloseReason = closed.Code, closed.Reason
		case errors.As(err, &fatal):
			event.CloseCode, event.CloseReason = fatal.Code, fatal.Reason
		}
		c.emitSynthetic(event)
	}()

	g.ackPending.Store(false)
	go g.heartbeat(ctx, c, interval)

	return g.readLoop(ctx, c)
}

// gatewayURL returns the URL to dial and whether the session can be resumed.
// Discord hands out a dedicated resume URL alongside READY; using anything
// else forfeits the session.
func (g *gateway) gatewayURL(c *Client) (string, bool) {
	if sid := c.sessionID.Load(); sid != nil && *sid != "" {
		if u := c.resumeURL.Load(); u != nil && *u != "" {
			return *u, true
		}
	}
	return "", false
}

// readHello reads the mandatory first frame and returns the heartbeat interval.
func (g *gateway) readHello(ctx context.Context) (time.Duration, error) {
	data, err := g.read(ctx)
	if err != nil {
		return 0, fmt.Errorf("reading hello: %w", err)
	}

	var frame struct {
		Op Opcode `json:"op"`
		D  struct {
			HeartbeatInterval int64 `json:"heartbeat_interval"`
		} `json:"d"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		return 0, fmt.Errorf("decoding hello: %w", err)
	}
	if frame.Op != OpHello {
		return 0, fmt.Errorf("expected hello (op %d), got op %d", OpHello, frame.Op)
	}
	return time.Duration(frame.D.HeartbeatInterval) * time.Millisecond, nil
}

// heartbeat sends a keep-alive every interval and tears the connection down if
// Discord stops acknowledging them.
//
// The first beat is delayed by a random fraction of the interval, as Discord
// asks, so that many bots reconnecting at once do not beat in unison.
func (g *gateway) heartbeat(ctx context.Context, c *Client, interval time.Duration) {
	jitter := time.Duration(rand.Float64() * float64(interval))
	timer := time.NewTimer(jitter)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if g.ackPending.Load() {
			// The last beat was never acknowledged: the connection is up as
			// far as TCP is concerned but Discord is not listening. Close it
			// with a non-1000 code so the session stays resumable.
			c.gatewayLogger().Warn("starlings: heartbeat not acknowledged, reconnecting")
			g.conn.Close(websocket.StatusCode(4000), "heartbeat timeout")
			return
		}

		g.ackPending.Store(true)
		if err := g.sendHeartbeat(ctx, c); err != nil {
			if ctx.Err() == nil {
				c.gatewayLogger().Warn("starlings: sending heartbeat", "err", err)
			}
			return
		}
		timer.Reset(interval)
	}
}

// sendHeartbeat sends op 1 with the last sequence number seen, which is how
// Discord knows what to replay on a resume.
func (g *gateway) sendHeartbeat(ctx context.Context, c *Client) error {
	g.beatSent.Store(time.Now().UnixNano())
	seq := c.seq.Load()
	if seq == 0 {
		return g.sendPriority(ctx, []byte(`{"op":1,"d":null}`))
	}
	buf := make([]byte, 0, 32)
	buf = append(buf, `{"op":1,"d":`...)
	buf = appendInt(buf, seq)
	buf = append(buf, '}')
	return g.sendPriority(ctx, buf)
}

// readLoop pumps frames until the connection fails.
func (g *gateway) readLoop(ctx context.Context, c *Client) error {
	for {
		data, err := g.read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var closeErr websocket.CloseError
			if errors.As(err, &closeErr) {
				cc := CloseCode(closeErr.Code)
				if cc.Fatal() {
					return &FatalError{Code: cc, Reason: closeErr.Reason}
				}
				if !cc.Resumable() {
					c.clearSession()
				}
				return &GatewayCloseError{Code: cc, Reason: closeErr.Reason}
			}
			return err
		}

		if err := g.handleFrame(ctx, c, data); err != nil {
			return err
		}
	}
}

// errReconnect asks the run loop for a fresh connection.
var errReconnect = errors.New("starlings: reconnect requested")

// handleFrame parses one gateway frame.
//
// This is the hot path, so it walks the JSON with a streaming decoder rather
// than unmarshalling into a struct. Discord puts "t", "s" and "op" ahead of
// "d", so by the time the payload is reached the frame's identity is known,
// and two shortcuts open up:
//
//   - An event with no registered handler is skipped without being parsed,
//     copied or allocated. Most of what the gateway sends is of no interest to
//     any given bot, so this is where the throughput comes from.
//   - An event that is wanted is decoded straight out of the decoder's own
//     buffer, with no intermediate copy of the payload.
//
// The second shortcut is why dispatch happens inside the parse loop rather
// than after it: a value read from the decoder is only valid until the next
// read from that same decoder.
func (g *gateway) handleFrame(ctx context.Context, c *Client, data []byte) error {
	dec := decoderPool.Get().(*jsontext.Decoder)
	defer decoderPool.Put(dec)
	g.rdr.Reset(data)
	// The option must be repeated here: Reset replaces the decoder's options
	// rather than preserving the ones NewDecoder was given.
	dec.Reset(&g.rdr, jsontext.AllowDuplicateNames(true))

	tok, err := dec.ReadToken()
	if err != nil {
		return fmt.Errorf("decoding frame: %w", err)
	}
	if tok.Kind() != '{' {
		return fmt.Errorf("decoding frame: expected an object, got %s", tok.Kind())
	}

	var (
		op         Opcode = -1
		name       string
		resumable  bool
		dispatched bool
		buffered   bool
	)
	g.scratch = g.scratch[:0]

	for {
		tok, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("decoding frame: %w", err)
		}
		if tok.Kind() == '}' {
			break
		}

		switch tok.String() {
		case "op":
			n, err := dec.ReadToken()
			if err != nil {
				return err
			}
			// Token.Int panics rather than erroring when the token is not a
			// number, so the kind has to be checked first. A frame with a
			// non-numeric op is malformed, and must produce an error rather
			// than take the process down.
			if n.Kind() != '0' {
				return fmt.Errorf("decoding frame: op is %s, expected a number", n.Kind())
			}
			v, err := n.Int()
			if err != nil {
				return err
			}
			op = Opcode(v)

		case "s":
			n, err := dec.ReadToken()
			if err != nil {
				return err
			}
			// null is normal - non-dispatch frames carry no sequence. Anything
			// other than null or a number is malformed; ignore it rather than
			// letting Token.Int panic.
			if n.Kind() == '0' {
				v, err := n.Int()
				if err != nil {
					return err
				}
				// Sequence numbers only move forward; a resume replays old
				// ones, so guard against rewinding.
				if v > c.seq.Load() {
					c.seq.Store(v)
				}
			}

		case "t":
			n, err := dec.ReadToken()
			if err != nil {
				return err
			}
			if n.Kind() != 'n' {
				name = n.String()
			}

		case "d":
			switch {
			case op == OpDispatch && name != "":
				if !c.wants(name) {
					// Nobody is listening: skip the payload untouched.
					if err := dec.SkipValue(); err != nil {
						return err
					}
					continue
				}
				payload, err := dec.ReadValue()
				if err != nil {
					return err
				}
				// Consumed here, before any further read invalidates it.
				c.dispatch(name, payload)
				dispatched = true

			case op == OpInvalidSession:
				payload, err := dec.ReadValue()
				if err != nil {
					return err
				}
				resumable = len(payload) > 0 && payload[0] == 't'

			case op == -1:
				// "d" arrived before "op". Discord does not normally do this,
				// but the value dies at the next read, so it has to be copied.
				payload, err := dec.ReadValue()
				if err != nil {
					return err
				}
				g.scratch = append(g.scratch, payload...)
				buffered = true

			default:
				if err := dec.SkipValue(); err != nil {
					return err
				}
			}

		default:
			if err := dec.SkipValue(); err != nil {
				return err
			}
		}
	}

	// Fall back to the buffered payload for the out-of-order case.
	if buffered {
		switch {
		case op == OpDispatch && name != "" && !dispatched:
			if c.wants(name) {
				c.dispatch(name, g.scratch)
			}
		case op == OpInvalidSession:
			resumable = len(g.scratch) > 0 && g.scratch[0] == 't'
		}
	}

	switch op {
	case OpDispatch:
		return nil

	case OpHeartbeat:
		// Discord can ask for an immediate beat instead of waiting for the
		// interval to elapse.
		return g.sendHeartbeat(ctx, c)

	case OpHeartbeatACK:
		g.ackPending.Store(false)
		if sent := g.beatSent.Load(); sent > 0 {
			g.latency.Store(time.Since(time.Unix(0, sent)).Nanoseconds())
		}
		return nil

	case OpReconnect:
		c.gatewayLogger().Info("starlings: gateway asked us to reconnect")
		return errReconnect

	case OpInvalidSession:
		if !resumable {
			c.clearSession()
		}
		c.gatewayLogger().Info("starlings: session invalidated", "resumable", resumable)
		// Discord asks for a short random pause before identifying again.
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(1000+rand.IntN(4000)) * time.Millisecond):
		}
		return errReconnect

	default:
		return nil
	}
}

// internalEvent reports whether starlings needs an event even when the bot has no
// handler for it. READY carries the session state that reconnecting depends
// on, so it is never skipped.
func internalEvent(name string) bool {
	return name == "READY" || name == "RESUMED"
}

// dispatch routes a decoded event to its handlers, after applying any session
// state it carries.
func (c *Client) dispatch(name string, payload jsontext.Value) {
	switch name {
	case "READY":
		c.applyReady(payload)
		c.gw.live.Store(true)
	case "RESUMED":
		c.gw.live.Store(true)
	}
	if handlers := c.rootClient().rawHandlers.Load(); len(*handlers) > 0 {
		data := append(jsontext.Value(nil), payload...)
		e := &RawEvent{Name: name, Data: data, Client: c}
		for _, h := range *handlers {
			if c.asyncEvents {
				go h.fn(e)
			} else {
				h.fn(e)
			}
		}
	}

	slot := c.slotFor(name)
	if slot == nil {
		return
	}
	slot.dispatch(c, payload)
}

// applyReady records the session ID, resume URL and identity from READY.
// It runs whether or not the bot registered a Ready handler.
func (c *Client) applyReady(payload jsontext.Value) {
	var r struct {
		User             *User  `json:"user"`
		SessionID        string `json:"session_id"`
		ResumeGatewayURL string `json:"resume_gateway_url"`
		Application      struct {
			ID Snowflake `json:"id"`
		} `json:"application"`
	}
	if err := json.Unmarshal(payload, &r); err != nil {
		c.gatewayLogger().Error("starlings: decoding READY", "err", err)
		return
	}

	c.sessionID.Store(&r.SessionID)
	c.resumeURL.Store(&r.ResumeGatewayURL)
	if r.User != nil {
		c.self.Store(r.User)
	}
	c.appID.Store(uint64(r.Application.ID))

	c.readyOnce.Do(func() { close(c.ready) })

	if r.User != nil {
		c.gatewayLogger().Info("starlings: connected", "user", r.User.Tag(), "id", r.User.ID)
	}
}

// clearSession forgets the session, forcing the next connection to identify
// afresh rather than resume.
func (c *Client) clearSession() {
	empty := ""
	c.sessionID.Store(&empty)
	c.resumeURL.Store(&empty)
	c.seq.Store(0)
}

type identifyPayload struct {
	Token      string             `json:"token"`
	Intents    Intent             `json:"intents"`
	Properties identityProperties `json:"properties"`
	Shard      *[2]int            `json:"shard,omitzero"`
	Presence   *presence          `json:"presence,omitzero"`
}

type identityProperties struct {
	OS      string `json:"os"`
	Browser string `json:"browser"`
	Device  string `json:"device"`
}

type resumePayload struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
}

func (c *Client) identifyPayload() identifyPayload {
	osName := c.id.OS
	if osName == "" {
		osName = runtimeOS()
	}
	p := identifyPayload{
		Token:      c.token,
		Intents:    c.intents,
		Properties: identityProperties{OS: osName, Browser: c.id.Browser, Device: c.id.Device},
		Presence:   c.initialState,
	}
	if c.shard[1] > 0 {
		p.Shard = &c.shard
	}
	return p
}

// send marshals a payload into an op frame and writes it.
func (g *gateway) send(ctx context.Context, op Opcode, d any) error {
	frame := struct {
		Op Opcode `json:"op"`
		D  any    `json:"d"`
	}{op, d}

	buf, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if op == OpIdentify || op == OpResume {
		return g.sendPriority(ctx, buf)
	}
	return g.sendRaw(ctx, buf)
}

// sendRaw writes one already-encoded frame, waiting if the connection has
// used its share of Discord's 120-commands-per-minute limit.
func (g *gateway) sendRaw(ctx context.Context, data []byte) error {
	if err := g.sends.take(ctx, false); err != nil {
		return err
	}
	return g.write(ctx, data)
}

// sendPriority is sendRaw for heartbeats, identify, and resume, which may use
// the slots other commands leave in reserve.
func (g *gateway) sendPriority(ctx context.Context, data []byte) error {
	if err := g.sends.take(ctx, true); err != nil {
		return err
	}
	return g.write(ctx, data)
}

// write sends one frame. The gateway allows no concurrent writes, so every
// send funnels through one mutex.
func (g *gateway) write(ctx context.Context, data []byte) error {
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	if g.conn == nil {
		return errNotReady
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return g.conn.Write(ctx, websocket.MessageText, data)
}

// decoderPool reuses gateway frame decoders. Every frame goes through one, so
// the saving compounds quickly on a busy bot.
var decoderPool = sync.Pool{
	New: func() any {
		// Discord never sends duplicate object members, so json/v2 need not
		// spend time proving they are absent. Profiling put that check at
		// ~6% of frame-parsing CPU.
		return jsontext.NewDecoder(bytes.NewReader(nil), jsontext.AllowDuplicateNames(true))
	},
}
