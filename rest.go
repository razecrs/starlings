package starlings

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// BaseURL is the root of Discord's REST API.
const BaseURL = "https://discord.com/api/v" + Version

// userAgent identifies the library to Discord, which their docs require.
const userAgent = "DiscordBot (https://github.com/razecrs/starlings, 0.1)"

// maxRetries caps how many times a single call is retried after a 429 or a
// server error before giving up.
const maxRetries = 3

const (
	maxRESTErrorBody = 1 << 20  // 1 MiB is ample for Discord error payloads.
	maxRESTResponse  = 64 << 20 // Match the gateway's generous frame ceiling.
)

var errRESTResponseTooLarge = errors.New("starlings: REST response body exceeds limit")

// rest is the HTTP half of a Client: one tuned http.Client, a rate limiter,
// a pacemaker, and a pool of buffers for request bodies.
type rest struct {
	c       *Client
	http    *http.Client
	limiter *limiter
	pacer   pacemaker
	bufs    sync.Pool

	// tokenRejected is set after Discord answers 401 to the bot token.
	// Further requests would only add to the invalid-request count.
	tokenRejected atomic.Bool
}

// ErrTokenRejected is returned for every bot-token request after Discord
// has answered one with 401 Unauthorized. Discord asks clients to stop using
// a rejected token, and repeated 401s count toward a temporary IP block.
var ErrTokenRejected = errors.New("starlings: Discord rejected the bot token; requests are stopped until the client is recreated")

// isInteractionRequest reports whether req answers an interaction: the
// callback route, or follow-up webhooks addressed by the application ID and
// interaction token.
func (r *rest) isInteractionRequest(req request) bool {
	if strings.HasPrefix(req.Path, "/interactions/") {
		return true
	}
	if !strings.HasPrefix(req.Path, "/webhooks/") {
		return false
	}
	app := r.c.rootClient().ApplicationID()
	return !app.IsZero() && strings.HasPrefix(req.Path, "/webhooks/"+app.String()+"/")
}

// countInvalid tracks responses that count toward Cloudflare's invalid
// request limit and stops bot-token requests after a 401.
func (r *rest) countInvalid(req request, resp *http.Response) {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
	default:
		return
	}
	if resp.StatusCode == http.StatusTooManyRequests && strings.EqualFold(resp.Header.Get("X-RateLimit-Scope"), "shared") {
		return // Discord does not count shared-resource limits.
	}
	if resp.StatusCode == http.StatusUnauthorized && req.Auth == "" && !req.NoAuth {
		if r.tokenRejected.CompareAndSwap(false, true) {
			r.c.log.Error("starlings: Discord rejected the bot token (401); stopping REST requests")
		}
	}
	if count, warn := r.limiter.invalid.add(time.Now()); warn {
		r.c.log.Warn("starlings: many invalid requests; Discord blocks the IP at 10,000 in ten minutes",
			"count", count, "window", invalidWindow)
	}
}

type preparedBody struct {
	data []byte
	file *os.File
	size int64
}

func (b preparedBody) reader() (io.Reader, int64, error) {
	if b.file == nil {
		if b.data == nil {
			return nil, 0, nil
		}
		return bytes.NewReader(b.data), int64(len(b.data)), nil
	}
	if _, err := b.file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	return io.LimitReader(b.file, b.size), b.size, nil
}

func newREST(c *Client) *rest {
	return &rest{
		c:       c,
		http:    defaultHTTPClient(),
		limiter: newLimiter(),
		bufs: sync.Pool{
			New: func() any { return new(bytes.Buffer) },
		},
	}
}

// request describes one REST call.
//
// Route is the rate-limit bucket key: the path with its major parameters
// (channel, guild and webhook IDs) substituted but everything else left as a
// placeholder, because Discord buckets requests that way. Getting it wrong
// costs correctness, not just speed, so the endpoint helpers always set it.
type request struct {
	Method string
	Path   string // the real path, appended to BaseURL
	Route  string // the rate-limit bucket key
	Body   any    // marshalled as JSON when non-nil
	Reason string // sets X-Audit-Log-Reason

	// Auth overrides the Authorization header. Empty means the bot token,
	// which is what almost every endpoint wants; the per-user OAuth2 endpoints
	// need a Bearer token instead.
	Auth   string
	NoAuth bool

	// Base overrides the API base URL. Empty means BaseURL. Some of the
	// undocumented social-layer endpoints exist only on v9.
	Base string

	// Files turns the request into a multipart upload, with Body carried
	// alongside as the "payload_json" part.
	Files       []File
	Headers     http.Header
	RawBody     []byte
	rawResponse bool

	// contentType is filled in by do once the body has been encoded.
	contentType string
}

// RESTRequest is the unrestricted REST escape hatch. Route should be the
// stable rate-limit bucket key; when empty Starlings uses "METHOD path".
type RESTRequest struct {
	Method      string
	Path        string
	Route       string
	Body        any
	RawBody     []byte
	Files       []File
	Reason      string
	Auth        string
	NoAuth      bool
	Base        string
	Headers     http.Header
	ContentType string
}

func (r RESTRequest) internal(rawResponse bool) request {
	route := r.Route
	if route == "" {
		route = r.Method + " " + r.Path
	}
	return request{Method: r.Method, Path: r.Path, Route: route, Body: r.Body,
		RawBody: r.RawBody, Files: r.Files, Reason: r.Reason, Auth: r.Auth, NoAuth: r.NoAuth,
		Base: r.Base, Headers: r.Headers, contentType: r.ContentType, rawResponse: rawResponse}
}

// Request performs an arbitrary Discord REST request and JSON-decodes out.
func (c *Client) Request(ctx context.Context, req RESTRequest, out any) error {
	return c.rest.do(ctx, req.internal(false), out)
}

// RequestRaw performs an arbitrary request and returns the response bytes.
func (c *Client) RequestRaw(ctx context.Context, req RESTRequest) ([]byte, error) {
	var out []byte
	err := c.rest.do(ctx, req.internal(true), &out)
	return out, err
}

// do performs a REST call, decoding the response into out when out is non-nil.
func (r *rest) do(ctx context.Context, req request, out any) error {
	if _, ok := ctx.Deadline(); !ok && r.c.requestTimeout > 0 {
		// A call made without a deadline, such as from a resource method,
		// still cannot hang forever on a stalled bucket or connection.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.c.requestTimeout)
		defer cancel()
	}
	var body preparedBody

	switch {
	case len(req.Files) > 0:
		// Attachments go as multipart, with the usual JSON body riding along
		// in a "payload_json" part.
		var err error
		body.file, body.size, req.contentType, err = multipartTempBody(req)
		if err != nil {
			return err
		}
		defer func() {
			name := body.file.Name()
			_ = body.file.Close()
			_ = os.Remove(name)
		}()

	case req.RawBody != nil:
		body.data = req.RawBody
	case req.Body != nil:
		buf := r.bufs.Get().(*bytes.Buffer)
		buf.Reset()
		defer r.bufs.Put(buf)

		if err := json.MarshalWrite(buf, req.Body); err != nil {
			return fmt.Errorf("starlings: encoding request body: %w", err)
		}
		body.data = buf.Bytes()
		req.contentType = "application/json"
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		retryAfter, err := r.attemptPrepared(ctx, req, body, out)
		if err == nil {
			return nil
		}
		lastErr = err

		if retryAfter <= 0 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryAfter):
		}
	}
	return lastErr
}

// attempt makes one HTTP round trip. A positive returned duration means the
// call is worth retrying after waiting that long.
func (r *rest) attempt(ctx context.Context, req request, body []byte, out any) (time.Duration, error) {
	return r.attemptPrepared(ctx, req, preparedBody{data: body}, out)
}

func (r *rest) attemptPrepared(ctx context.Context, req request, body preparedBody, out any) (time.Duration, error) {
	// Interaction callbacks and follow-ups have a three-second deadline and
	// are exempt from Discord's global limit, so they skip the pacer and the
	// global wait. Their own route buckets still apply.
	interaction := r.isInteractionRequest(req)
	botAuth := req.Auth == "" && !req.NoAuth
	if botAuth && r.tokenRejected.Load() {
		return 0, ErrTokenRejected
	}
	if !interaction {
		if err := r.pacer.wait(ctx); err != nil {
			return 0, err
		}
	}
	b := r.limiter.bucketFor(req.Route)
	waited, err := b.waitFor(ctx)
	if err != nil {
		return 0, err
	}
	defer b.settle()
	if waited >= time.Millisecond {
		r.c.emitSynthetic(&RateLimitWait{Method: req.Method, Route: req.Route, WaitSeconds: waited.Seconds()})
	}
	if !interaction && botAuth {
		if err := r.limiter.global.take(ctx); err != nil {
			return 0, err
		}
		if err := r.limiter.waitGlobal(ctx); err != nil {
			return 0, err
		}
	}

	rdr, contentLength, err := body.reader()
	if err != nil {
		return 0, fmt.Errorf("starlings: rewinding request body: %w", err)
	}

	base := BaseURL
	if req.Base != "" {
		base = req.Base
	}
	if !req.NoAuth && req.Auth == "" && !trustedDiscordAPIBase(base) {
		return 0, errors.New("starlings: refusing to send the client token to an untrusted API base; use NoAuth or an explicit Auth value")
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, base+req.Path, rdr)
	if err != nil {
		// url.Error and url.ParseError may echo the complete URL, including a
		// webhook credential embedded in its path. The method and sanitized path
		// are enough to diagnose malformed request construction safely.
		return 0, fmt.Errorf("starlings: building %s %s: invalid request URL", req.Method, safeRequestPath(req.Path))
	}
	auth := r.c.token
	if req.Auth != "" {
		auth = req.Auth
	}
	if !req.NoAuth {
		httpReq.Header.Set("Authorization", auth)
	}
	ua := r.c.id.UserAgent
	if ua == "" {
		ua = userAgent
	}
	httpReq.Header.Set("User-Agent", ua)
	if rdr != nil {
		httpReq.ContentLength = contentLength
	}
	if rdr != nil && req.contentType != "" {
		httpReq.Header.Set("Content-Type", req.contentType)
	}
	if req.Reason != "" {
		httpReq.Header.Set("X-Audit-Log-Reason", req.Reason)
	}
	for key, values := range req.Headers {
		httpReq.Header.Del(key)
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}

	resp, err := r.http.Do(httpReq)
	if err != nil {
		// A transport error is usually transient, so it is worth one more go.
		transportErr := err
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			// url.Error includes the complete request URL, which may contain a
			// webhook or interaction credential. Keep only its wrapped cause.
			transportErr = urlErr.Err
		}
		return time.Second, fmt.Errorf("starlings: %s %s: %w", req.Method, safeRequestPath(req.Path), transportErr)
	}
	defer resp.Body.Close()

	b.update(resp.Header)
	r.limiter.share(req.Route, b, resp.Header)
	r.countInvalid(req, resp)

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		data, err := readLimitedBody(resp.Body, maxRESTErrorBody)
		if err != nil {
			return 0, fmt.Errorf("starlings: reading rate-limit response: %w", err)
		}
		wait, global := r.limiter.handle429(resp, data)
		r.c.log.Warn("starlings: rate limited", "route", req.Route, "wait", wait)
		r.c.emitSynthetic(&RateLimit{
			Method: req.Method, Path: safeRequestPath(req.Path), Route: req.Route,
			RetryAfterSeconds: wait.Seconds(), Global: global,
		})
		apiErr := parseAPIError(resp.StatusCode, data)
		if api, ok := apiErr.(*APIError); ok && api.Message == "" {
			api.Message = "rate limited"
		}
		return wait, apiErr

	case resp.StatusCode >= 500:
		data, err := readLimitedBody(resp.Body, maxRESTErrorBody)
		if err != nil {
			return 0, fmt.Errorf("starlings: reading server-error response: %w", err)
		}
		return time.Duration(1<<uint(resp.StatusCode%2)) * time.Second,
			&APIError{Status: resp.StatusCode, Body: data, Message: "server error"}

	case resp.StatusCode >= 400:
		data, err := readLimitedBody(resp.Body, maxRESTErrorBody)
		if err != nil {
			return 0, fmt.Errorf("starlings: reading API-error response: %w", err)
		}
		return 0, parseAPIError(resp.StatusCode, data)

	case resp.StatusCode == http.StatusNoContent || out == nil:
		_, _ = io.Copy(io.Discard, resp.Body)
		return 0, nil
	}

	if req.rawResponse {
		data, err := readLimitedBody(resp.Body, maxRESTResponse)
		if err != nil {
			return 0, fmt.Errorf("starlings: reading %s %s: %w", req.Method, safeRequestPath(req.Path), err)
		}
		*out.(*[]byte) = data
		return 0, nil
	}
	if err := decodeLimitedJSON(resp.Body, maxRESTResponse, out); err != nil {
		if errors.Is(err, errRESTResponseTooLarge) {
			return 0, fmt.Errorf("starlings: reading %s %s: %w", req.Method, safeRequestPath(req.Path), err)
		}
		return 0, fmt.Errorf("starlings: decoding %s %s: %w", req.Method, safeRequestPath(req.Path), err)
	}
	bindResult(r.c, out, guildFromPath(req.Path))
	return 0, nil
}

// decodeLimitedJSON decodes directly from the response stream. UnmarshalRead
// consumes through EOF, so the extra byte in the limited reader preserves the
// hard response cap without first copying the entire payload into []byte.
func decodeLimitedJSON(r io.Reader, limit int64, out any) error {
	lr := &io.LimitedReader{R: r, N: limit + 1}
	err := json.UnmarshalRead(lr, out)
	if lr.N == 0 {
		return errRESTResponseTooLarge
	}
	return err
}

func readLimitedBody(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errRESTResponseTooLarge
	}
	return data, nil
}

func trustedDiscordAPIBase(base string) bool {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "discord.com" || strings.HasSuffix(host, ".discord.com") ||
		host == "discordapp.com" || strings.HasSuffix(host, ".discordapp.com")
}

// safeRequestPath removes credentials Discord places in webhook and
// interaction URLs before a path reaches an error, log, or synthetic event.
func safeRequestPath(path string) string {
	parsed, err := url.Parse(path)
	if err != nil {
		return "[redacted path]"
	}
	parts := strings.Split(parsed.Path, "/")
	for i := 1; i+2 < len(parts); i++ {
		if (parts[i] == "webhooks" || parts[i] == "interactions") && parts[i+1] != "" {
			parts[i+2] = "[redacted]"
		}
	}
	parsed.Path = strings.Join(parts, "/")
	query := parsed.Query()
	for key := range query {
		switch strings.ToLower(key) {
		case "token", "access_token", "client_secret", "signature", "key":
			query.Set(key, "[redacted]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// parseAPIError turns Discord's error body into an APIError, keeping the raw
// bytes for anything the struct does not model.
func parseAPIError(status int, data []byte) error {
	e := &APIError{Status: status, Body: data}

	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &payload); err == nil {
		e.Code = payload.Code
		e.Message = payload.Message
	}
	return e
}

// Rate limits

// pacemaker enforces a minimum interval between the starts of any two REST
// requests. It is global - one pace for the whole client, not per bucket -
// and it serialises: with a gap set, requests go out one at a time, which is
// what WithPacing uses to keep a job from going out as a burst.
type pacemaker struct {
	mu   sync.Mutex
	gap  time.Duration
	last time.Time
}

// set changes the minimum interval. Zero means no pacing.
func (p *pacemaker) set(gap time.Duration) {
	p.mu.Lock()
	p.gap = gap
	p.mu.Unlock()
}

// Gap reports the current minimum interval.
func (p *pacemaker) Gap() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gap
}

// wait blocks until at least gap has passed since the previous request
// started, then claims this slot. The lock is held across the wait on
// purpose: paced requests queue and go out one at a time.
func (p *pacemaker) wait(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.gap > 0 {
		if d := time.Until(p.last.Add(p.gap)); d > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
	}
	p.last = time.Now()
	return nil
}
