package starlings

import (
	"context"

	"github.com/coder/websocket"
)

// defaultGatewayURL is the address Discord's /gateway/bot returns in
// practice. Dialing it while that request is in flight saves a round of TCP,
// TLS, and websocket setup at startup.
const defaultGatewayURL = "wss://gateway.discord.gg"

// preDial is a gateway connection opened before the client knows whether it
// will use it.
type preDial struct {
	url  string
	done chan struct{}
	conn *websocket.Conn
	err  error
}

func (c *Client) gatewayDialURL(base string) string {
	query := "/?v=" + Version + "&encoding=json"
	if c.compress {
		query += "&compress=zlib-stream"
	}
	return base + query
}

// startPreDial opens a connection to url in the background. Nothing is sent
// on it, and Discord's HELLO waits in the socket until the connection is
// adopted, so identifying still happens only after the shard count is known.
func (c *Client) startPreDial(ctx context.Context, base string) *preDial {
	p := &preDial{url: c.gatewayDialURL(base), done: make(chan struct{})}
	go func() {
		defer close(p.done)
		p.conn, _, p.err = websocket.Dial(ctx, p.url, &websocket.DialOptions{HTTPClient: c.rest.http})
	}()
	return p
}

// take returns the connection if it was opened to url, or nil.
func (p *preDial) take(ctx context.Context, url string) *websocket.Conn {
	select {
	case <-p.done:
	case <-ctx.Done():
		p.discard()
		return nil
	}
	if p.err != nil || p.url != url {
		p.discard()
		return nil
	}
	return p.conn
}

// discard closes the connection once it is open, without waiting for it.
func (p *preDial) discard() {
	go func() {
		<-p.done
		if p.conn != nil {
			p.conn.CloseNow()
		}
	}()
}
