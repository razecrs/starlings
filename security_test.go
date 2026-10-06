package starlings

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type failingRoundTripper struct{ err error }

func (f failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestRESTRefusesClientTokenForUntrustedBase(t *testing.T) {
	c := New("super-secret-token", WithLogger(discardLogger()))
	_, err := c.RequestRaw(context.Background(), RESTRequest{
		Method: http.MethodGet,
		Base:   "https://attacker.invalid",
		Path:   "/collect",
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to send") {
		t.Fatalf("untrusted authenticated base error = %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("refusal leaked token: %v", err)
	}
}

func TestUntrustedTerminalTextIsMadeVisible(t *testing.T) {
	input := "hello\x1b]52;c;stolen\a\x1b[2J\nspoof\u202eexe"
	got := sanitizeUntrustedText(input)
	for _, unsafe := range []string{"\x1b", "\a", "\n", "\u202e"} {
		if strings.Contains(got, unsafe) {
			t.Fatalf("sanitized text retains unsafe rune %q: %q", unsafe, got)
		}
	}
	for _, visible := range []string{`\x1b`, `\u0007`, `\n`, `\u202e`} {
		if !strings.Contains(got, visible) {
			t.Fatalf("sanitized text does not expose %q: %q", visible, got)
		}
	}
}

func TestStarlogSanitizesMessagesAndFields(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("test", StarlogTUI(false), StarlogColor(false),
		StarlogWithPet(StarlogPet{}), StarlogOutput(&output))
	log.Logger().LogAttrs(context.Background(), slog.LevelInfo, "remote\x1b[2J", slog.String("name", "x\nspoof"))
	got := output.String()
	if strings.Contains(got, "\x1b") || strings.Contains(got, "x\nspoof") {
		t.Fatalf("starlog emitted terminal controls: %q", got)
	}
	if !strings.Contains(got, `remote\x1b[2J`) || !strings.Contains(got, `x\nspoof`) {
		t.Fatalf("starlog did not render controls visibly: %q", got)
	}
}

func TestRemoteErrorsSanitizeTerminalControls(t *testing.T) {
	err := (&APIError{Status: 400, Message: "bad\x1b[2J", Errors: []FieldError{{Path: "x\u202e", Message: "no\nway"}}}).Error()
	fatal := (&FatalError{Code: CloseAuthenticationFailed, Reason: "bad\x1b]52;c;x\a"}).Error()
	for name, got := range map[string]string{"api": err, "gateway": fatal} {
		if strings.ContainsAny(got, "\x1b\a\u202e") {
			t.Fatalf("%s error retained terminal controls: %q", name, got)
		}
	}
}

func TestRESTAllowsExplicitAuthOrNoAuthForCustomBase(t *testing.T) {
	for _, req := range []RESTRequest{
		{Method: http.MethodGet, Base: "https://custom.invalid", Path: "/x", NoAuth: true},
		{Method: http.MethodGet, Base: "https://custom.invalid", Path: "/x", Auth: "Bearer explicit"},
	} {
		c := New("client-token", WithLogger(discardLogger()), WithHTTPClient(&http.Client{
			Transport: failingRoundTripper{err: errors.New("expected transport stop")},
		}))
		_, err := c.rest.attempt(context.Background(), req.internal(true), nil, new([]byte))
		if err == nil || !strings.Contains(err.Error(), "expected transport stop") {
			t.Fatalf("custom request error = %v", err)
		}
	}
}

func TestSensitiveRequestPathsAreRedacted(t *testing.T) {
	const secret = "this-is-a-webhook-secret"
	for _, path := range []string{
		"/webhooks/123/" + secret + "/messages/456?thread_id=7",
		"/interactions/123/" + secret + "/callback",
		"/anything?access_token=" + url.QueryEscape(secret),
	} {
		got := safeRequestPath(path)
		if strings.Contains(got, secret) {
			t.Fatalf("safeRequestPath(%q) leaked its credential: %q", path, got)
		}
	}
}

func TestTransportErrorsDoNotLeakWebhookToken(t *testing.T) {
	const secret = "this-is-a-webhook-secret"
	c := New("client-token", WithLogger(discardLogger()), WithHTTPClient(&http.Client{
		Transport: failingRoundTripper{err: errors.New("network down")},
	}))
	req := RESTRequest{
		Method: http.MethodPost,
		Path:   "/webhooks/123/" + secret,
		NoAuth: true,
	}
	_, err := c.rest.attempt(context.Background(), req.internal(true), nil, new([]byte))
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("transport error leaked webhook token: %v", err)
	}
}

func TestMalformedRequestURLDoesNotLeakWebhookToken(t *testing.T) {
	const secret = "this-is-a-webhook-secret"
	c := New("client-token", WithLogger(discardLogger()))
	req := RESTRequest{Method: http.MethodPost, Path: "/webhooks/123/" + secret + "\x7f", NoAuth: true}
	_, err := c.rest.attempt(context.Background(), req.internal(true), nil, new([]byte))
	if err == nil {
		t.Fatal("expected malformed URL error")
	}
	if strings.Contains(err.Error(), secret) || strings.ContainsRune(err.Error(), '\x7f') {
		t.Fatalf("request construction error leaked unsafe URL data: %v", err)
	}
}

func TestNilHTTPClientOptionKeepsSafeDefault(t *testing.T) {
	c := New("token", WithHTTPClient(nil))
	if c.rest.http == nil {
		t.Fatal("WithHTTPClient(nil) disabled the HTTP client")
	}
}

func TestRESTResponseBodyLimit(t *testing.T) {
	_, err := readLimitedBody(strings.NewReader(strings.Repeat("x", 65)), 64)
	if !errors.Is(err, errRESTResponseTooLarge) {
		t.Fatalf("oversized response error = %v", err)
	}
	got, err := readLimitedBody(io.LimitReader(strings.NewReader("safe"), 4), 4)
	if err != nil || string(got) != "safe" {
		t.Fatalf("response at limit = %q, %v", got, err)
	}
}

func TestRESTJSONResponseBodyLimit(t *testing.T) {
	var atLimit string
	if err := decodeLimitedJSON(strings.NewReader(`"`+strings.Repeat("x", 62)+`"`), 64, &atLimit); err != nil {
		t.Fatalf("JSON response at limit: %v", err)
	}
	if len(atLimit) != 62 {
		t.Fatalf("decoded string length = %d, want 62", len(atLimit))
	}

	var oversized string
	err := decodeLimitedJSON(strings.NewReader(`"`+strings.Repeat("x", 63)+`"`), 64, &oversized)
	if !errors.Is(err, errRESTResponseTooLarge) {
		t.Fatalf("oversized JSON response error = %v", err)
	}

	var malformed any
	if err := decodeLimitedJSON(strings.NewReader(`{"broken":`), 64, &malformed); err == nil {
		t.Fatal("malformed JSON response was accepted")
	}
}
