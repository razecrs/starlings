package starlings

import (
	"context"
	"net/http"
	"net/url"
)

// BulkDeleteMessages removes 2 to 100 messages in one call.
//
// Discord refuses messages older than two weeks, and refuses a list of one -
// use DeleteMessage for a single message. Duplicated IDs also fail.
func (c *Client) BulkDeleteMessages(ctx context.Context, channelID Snowflake, ids []Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/messages/bulk-delete",
		Route:  "POST /channels/" + channelID.String() + "/messages/bulk-delete",
		Body:   map[string]any{"messages": ids},
		Reason: reason,
	}, nil)
}

// Crosspost publishes a message from an announcement channel to every server
// following it.
func (c *Client) Crosspost(ctx context.Context, channelID, messageID Snowflake) (*Message, error) {
	var out Message
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/messages/" + messageID.String() + "/crosspost",
		Route:  "POST /channels/" + channelID.String() + "/messages/{id}/crosspost",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ReactionsQuery pages through the users who reacted with one emoji.
type ReactionsQuery struct {
	After Snowflake
	Limit int // 1-100
	Type  int // 0 normal, 1 burst ("super") reactions
}

// Reactions lists who reacted to a message with a particular emoji.
func (c *Client) Reactions(ctx context.Context, channelID, messageID Snowflake, emoji Emoji, q ReactionsQuery) ([]User, error) {
	v := url.Values{}
	if !q.After.IsZero() {
		v.Set("after", q.After.String())
	}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}
	if q.Type > 0 {
		v.Set("type", itoa(q.Type))
	}

	var out []User
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/channels/" + channelID.String() + "/messages/" + messageID.String() +
			"/reactions/" + url.PathEscape(emoji.APIFormat()) + query(v),
		Route: "GET /channels/" + channelID.String() + "/messages/{id}/reactions/{emoji}",
	}, &out)
	return out, err
}

// RemoveUserReaction deletes someone else's reaction. Needs Manage Messages.
func (c *Client) RemoveUserReaction(ctx context.Context, channelID, messageID, userID Snowflake, emoji Emoji) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path: "/channels/" + channelID.String() + "/messages/" + messageID.String() +
			"/reactions/" + url.PathEscape(emoji.APIFormat()) + "/" + userID.String(),
		Route: "DELETE /channels/" + channelID.String() + "/messages/{id}/reactions/{emoji}/{id}",
	}, nil)
}

// ClearReactions removes every reaction from a message.
func (c *Client) ClearReactions(ctx context.Context, channelID, messageID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + channelID.String() + "/messages/" + messageID.String() + "/reactions",
		Route:  "DELETE /channels/" + channelID.String() + "/messages/{id}/reactions",
	}, nil)
}

// ClearReactionsFor removes every reaction of one emoji from a message.
func (c *Client) ClearReactionsFor(ctx context.Context, channelID, messageID Snowflake, emoji Emoji) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path: "/channels/" + channelID.String() + "/messages/" + messageID.String() +
			"/reactions/" + url.PathEscape(emoji.APIFormat()),
		Route: "DELETE /channels/" + channelID.String() + "/messages/{id}/reactions/{emoji}",
	}, nil)
}

// MessagePins is the newer paginated pins API, which supersedes the older
// /channels/{id}/pins endpoint and returns pin timestamps.
type MessagePins struct {
	Items   []MessagePin `json:"items"`
	HasMore bool         `json:"has_more"`
}

// MessagePin is one pinned message with the time it was pinned.
type MessagePin struct {
	PinnedAt string   `json:"pinned_at"`
	Message  *Message `json:"message"`
}

// PinnedMessages lists a channel's pins, newest pin first.
func (c *Client) PinnedMessages(ctx context.Context, channelID Snowflake, before string, limit int) (*MessagePins, error) {
	v := url.Values{}
	if before != "" {
		v.Set("before", before)
	}
	if limit > 0 {
		v.Set("limit", itoa(limit))
	}

	var out MessagePins
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String() + "/messages/pins" + query(v),
		Route:  "GET /channels/" + channelID.String() + "/messages/pins",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PinMessageV2 pins a message through the newer pins endpoint.
func (c *Client) PinMessageV2(ctx context.Context, channelID, messageID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/channels/" + channelID.String() + "/messages/pins/" + messageID.String(),
		Route:  "PUT /channels/" + channelID.String() + "/messages/pins/{id}",
		Reason: reason,
	}, nil)
}

// UnpinMessageV2 unpins a message through the newer pins endpoint.
func (c *Client) UnpinMessageV2(ctx context.Context, channelID, messageID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + channelID.String() + "/messages/pins/" + messageID.String(),
		Route:  "DELETE /channels/" + channelID.String() + "/messages/pins/{id}",
		Reason: reason,
	}, nil)
}

// MyPrivateArchivedThreads lists archived private threads the bot has joined.
func (c *Client) MyPrivateArchivedThreads(ctx context.Context, channelID Snowflake, q ArchivedThreadsQuery) ([]Channel, bool, error) {
	var out archivedThreadPage
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/channels/" + channelID.String() +
			"/users/@me/threads/archived/private" + archiveQuery(q),
		Route: "GET /channels/" + channelID.String() + "/users/@me/threads/archived/private",
	}, &out)
	return out.Threads, out.HasMore, err
}

// SetVoiceStatus sets the status line shown on a voice channel.
func (c *Client) SetVoiceStatus(ctx context.Context, channelID Snowflake, status string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/channels/" + channelID.String() + "/voice-status",
		Route:  "PUT /channels/" + channelID.String() + "/voice-status",
		Body:   map[string]any{"status": status},
	}, nil)
}
