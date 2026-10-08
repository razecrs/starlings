package starlings

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strings"
)

// MessageHistory walks a channel's history from newest to oldest, fetching
// 100 messages per request. Start at before, or at the newest message when
// before is zero. Stop early by breaking out of the loop; no further requests
// are made.
//
//	for msg, err := range bot.MessageHistory(ctx, channelID, 0) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(msg.Content)
//	}
//
// An error ends the iteration after it is yielded.
func (c *Client) MessageHistory(ctx context.Context, channelID, before Snowflake) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		for {
			page, err := c.Messages(ctx, channelID, MessagesQuery{Limit: 100, Before: before})
			if err != nil {
				yield(Message{}, err)
				return
			}
			for _, msg := range page {
				if !yield(msg, nil) {
					return
				}
			}
			if len(page) < 100 {
				return
			}
			before = page[len(page)-1].ID
		}
	}
}

// ErrUntrustedAttachmentURL is returned by DownloadAttachment for a URL that
// is not on Discord's CDN.
var ErrUntrustedAttachmentURL = errors.New("starlings: attachment URL is not on Discord's CDN")

// DownloadAttachment reads an attachment's bytes, up to limit. It only fetches
// from Discord's CDN hosts, so a URL taken from user input cannot make the bot
// request internal addresses. A limit of zero or less uses the attachment's
// reported size, plus a small margin.
func (c *Client) DownloadAttachment(ctx context.Context, a Attachment, limit int64) ([]byte, error) {
	raw := a.URL
	if raw == "" {
		raw = a.ProxyURL
	}
	if !trustedDiscordCDN(raw) {
		return nil, ErrUntrustedAttachmentURL
	}
	if limit <= 0 {
		limit = int64(a.Size) + 1024
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("starlings: building attachment request: %w", err)
	}
	ua := c.id.UserAgent
	if ua == "" {
		ua = userAgent
	}
	req.Header.Set("User-Agent", ua)
	// Validate every hop, not only the initial URL. Share the transport for
	// pooling, but do not change the application's client or redirect policy.
	downloadClient := *c.rest.http
	redirectPolicy := downloadClient.CheckRedirect
	downloadClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !trustedDiscordCDN(next.URL.String()) {
			return ErrUntrustedAttachmentURL
		}
		if redirectPolicy != nil {
			return redirectPolicy(next, via)
		}
		if len(via) >= 10 {
			return errors.New("starlings: too many attachment redirects")
		}
		return nil
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			// The signed CDN URL is a credential of sorts; keep it out of logs.
			err = urlErr.Err
		}
		return nil, fmt.Errorf("starlings: downloading attachment %s: %w", a.Filename, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("starlings: downloading attachment %s: HTTP %d", a.Filename, resp.StatusCode)
	}
	data, err := readLimitedBody(resp.Body, limit)
	if errors.Is(err, errRESTResponseTooLarge) {
		return nil, fmt.Errorf("starlings: attachment %s is larger than %d bytes", a.Filename, limit)
	}
	return data, err
}

func trustedDiscordCDN(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "cdn.discordapp.com", "media.discordapp.net":
		return true
	}
	return false
}
