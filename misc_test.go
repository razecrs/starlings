package starlings

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestGatewayUsesPublicUnauthenticatedEndpoint(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/v"+Version+"/gateway" {
			t.Fatalf("path = %q, want public gateway endpoint", request.URL.Path)
		}
		if auth := request.Header.Get("Authorization"); auth != "" {
			t.Fatalf("public gateway request sent Authorization: %q", auth)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"url":"wss://gateway.discord.gg"}`)),
			Request:    request,
		}, nil
	})
	c := New(WithToken("secret-token"), WithHTTPClient(&http.Client{Transport: transport}), WithLogger(discardLogger()))

	got, err := c.Gateway(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "wss://gateway.discord.gg" {
		t.Fatalf("Gateway() = %q", got)
	}
}
