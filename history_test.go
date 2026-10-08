package starlings

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// clientServedBy returns a client whose REST requests are answered in process
// by handler, with Discord's real base URL left in place.
func clientServedBy(handler http.Handler, opts ...Option) *Client {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, r)
		return recorder.Result(), nil
	})
	base := []Option{WithHTTPClient(&http.Client{Transport: transport}), WithLogger(discardLogger())}
	return New(append(append([]Option{WithToken("token")}, base...), opts...)...)
}

func TestMessageHistoryPagesUntilShortPage(t *testing.T) {
	var befores []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		before := r.URL.Query().Get("before")
		befores = append(befores, before)
		count := 100
		start := 1000
		if before != "" {
			count = 30
			start = 900
		}
		var parts []string
		for n := range count {
			parts = append(parts, fmt.Sprintf(`{"id":"%d","channel_id":"5","content":"m"}`, start-n))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", strings.Join(parts, ","))
	})

	c := clientServedBy(handler)
	seen := 0
	for _, err := range c.MessageHistory(context.Background(), 5, 0) {
		if err != nil {
			t.Fatal(err)
		}
		seen++
	}
	if seen != 130 || len(befores) != 2 || befores[0] != "" || befores[1] != "901" {
		t.Fatalf("seen %d messages over requests %q; want 130 over two pages, second before 901", seen, befores)
	}
}

func TestMessageHistoryStopsWhenLoopBreaks(t *testing.T) {
	requests := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var parts []string
		for n := range 100 {
			parts = append(parts, fmt.Sprintf(`{"id":"%d"}`, 1000-n))
		}
		fmt.Fprintf(w, "[%s]", strings.Join(parts, ","))
	})

	c := clientServedBy(handler)
	for range c.MessageHistory(context.Background(), 5, 0) {
		break
	}
	if requests != 1 {
		t.Fatalf("breaking after one message made %d requests, want 1", requests)
	}
}

func TestDownloadAttachmentRejectsNonCDNHosts(t *testing.T) {
	c := New(WithToken("token"))
	for _, raw := range []string{
		"http://cdn.discordapp.com/a.png",
		"https://169.254.169.254/latest/meta-data",
		"https://cdn.discordapp.com.evil.test/a.png",
		"https://user@cdn.discordapp.com/a.png",
		"https://cdn.discordapp.com:8443/a.png",
		"",
	} {
		if _, err := c.DownloadAttachment(context.Background(), Attachment{URL: raw}, 10); !errors.Is(err, ErrUntrustedAttachmentURL) {
			t.Errorf("DownloadAttachment(%q) = %v, want ErrUntrustedAttachmentURL", raw, err)
		}
	}
}
