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
	"strconv"
	"strings"
	"sync"
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
	if err := r.pacer.wait(ctx); err != nil {
		return 0, err
	}
	b := r.limiter.bucketFor(req.Route)
	if err := b.wait(ctx); err != nil {
		return 0, err
	}
	if err := r.limiter.waitGlobal(ctx); err != nil {
		return 0, err
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
	data, err := readLimitedBody(resp.Body, maxRESTResponse)
	if err != nil {
		return 0, fmt.Errorf("starlings: reading %s %s: %w", req.Method, safeRequestPath(req.Path), err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return 0, fmt.Errorf("starlings: decoding %s %s: %w", req.Method, safeRequestPath(req.Path), err)
	}
	return 0, nil
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

// gatewayURL asks Discord where to connect. It also reports the session start
// limits, which are logged when they run low.
func (r *rest) gatewayURL(ctx context.Context) (string, error) {
	var resp struct {
		URL               string `json:"url"`
		Shards            int    `json:"shards"`
		SessionStartLimit struct {
			Total          int `json:"total"`
			Remaining      int `json:"remaining"`
			ResetAfter     int `json:"reset_after"`
			MaxConcurrency int `json:"max_concurrency"`
		} `json:"session_start_limit"`
	}
	path, route := "/gateway/bot", "GET /gateway/bot"
	if r.c.userToken {
		// User tokens use the plain gateway endpoint; /gateway/bot reports
		// the bot's session starts, which a user account doesn't have.
		path, route = "/gateway", "GET /gateway"
	}
	err := r.do(ctx, request{
		Method: http.MethodGet,
		Path:   path,
		Route:  route,
	}, &resp)
	if err != nil {
		return "", err
	}

	if resp.SessionStartLimit.Remaining < 10 {
		r.c.log.Warn("starlings: few gateway sessions left today",
			"remaining", resp.SessionStartLimit.Remaining,
			"total", resp.SessionStartLimit.Total,
			"resets_in", time.Duration(resp.SessionStartLimit.ResetAfter)*time.Millisecond)
	}
	return resp.URL, nil
}

// Rate limits

// pacemaker enforces a minimum interval between the starts of any two REST
// requests. It is global - one pace for the whole client, not per bucket -
// and it serialises: with a gap set, requests go out one at a time, which is
// what keeps a selfbot from looking like a burst of requests.
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

// limiter tracks Discord's rate limits: one bucket per route, plus a global
// cap that applies across all of them.
type limiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket

	// globalUntil is set when Discord returns a global 429; every request
	// waits for it.
	globalMu    sync.Mutex
	globalUntil time.Time
}

func newLimiter() *limiter {
	return &limiter{buckets: make(map[string]*bucket, 16)}
}

func (l *limiter) bucketFor(route string) *bucket {
	l.mu.RLock()
	b := l.buckets[route]
	l.mu.RUnlock()
	if b != nil {
		return b
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if b = l.buckets[route]; b != nil {
		return b
	}
	b = &bucket{remaining: -1} // -1 means "not yet known"
	l.buckets[route] = b
	return b
}

// waitGlobal blocks while a global rate limit is in force.
func (l *limiter) waitGlobal(ctx context.Context) error {
	l.globalMu.Lock()
	until := l.globalUntil
	l.globalMu.Unlock()

	d := time.Until(until)
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// handle429 reads the retry delay from a 429 response and, if the limit was
// global, records it so every other request waits too.
func (l *limiter) handle429(resp *http.Response, body []byte) (time.Duration, bool) {
	wait := time.Second
	global := strings.EqualFold(resp.Header.Get("X-RateLimit-Global"), "true")
	var payload struct {
		RetryAfter float64 `json:"retry_after"`
		Global     bool    `json:"global"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		if payload.RetryAfter > 0 {
			wait = time.Duration(payload.RetryAfter * float64(time.Second))
		}
		global = global || payload.Global
	}

	if payload.RetryAfter <= 0 {
		if v := resp.Header.Get("Retry-After"); v != "" {
			if secs, err := strconv.ParseFloat(v, 64); err == nil {
				wait = time.Duration(secs * float64(time.Second))
			}
		}
	}

	if global {
		l.globalMu.Lock()
		if until := time.Now().Add(wait); until.After(l.globalUntil) {
			l.globalUntil = until
		}
		l.globalMu.Unlock()
	}
	return wait, global
}

// bucket is the state of one Discord rate-limit bucket.
type bucket struct {
	mu        sync.Mutex
	limit     int
	remaining int // -1 until the first response tells us
	resetAt   time.Time
}

// wait blocks until the bucket has room, then claims one request from it.
//
// The lock is held only for the accounting, not across the HTTP call, so
// several requests in a bucket with headroom still go out concurrently.
func (b *bucket) wait(ctx context.Context) error {
	for {
		b.mu.Lock()
		if b.remaining != 0 {
			if b.remaining > 0 {
				b.remaining--
			}
			b.mu.Unlock()
			return nil
		}
		d := time.Until(b.resetAt)
		b.mu.Unlock()

		if d <= 0 {
			// The window has passed; let the next response refresh the count.
			b.mu.Lock()
			if b.remaining == 0 && time.Now().After(b.resetAt) {
				b.remaining = -1
			}
			b.mu.Unlock()
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
}

// update refreshes the bucket from a response's rate-limit headers.
func (b *bucket) update(h http.Header) {
	remaining := h.Get("X-RateLimit-Remaining")
	resetAfter := h.Get("X-RateLimit-Reset-After")
	if remaining == "" && resetAfter == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if v, err := strconv.Atoi(h.Get("X-RateLimit-Limit")); err == nil {
		b.limit = v
	}
	if v, err := strconv.Atoi(remaining); err == nil {
		b.remaining = v
	}
	if v, err := strconv.ParseFloat(resetAfter, 64); err == nil {
		b.resetAt = time.Now().Add(time.Duration(v * float64(time.Second)))
	}
}
