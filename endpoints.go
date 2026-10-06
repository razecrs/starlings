package starlings

import (
	"context"
	"net/http"
	"net/url"
)

// SendData is everything a message can carry. Most calls need only Content, so
// prefer Client.Send and the Reply helpers; reach for this when you need
// embeds, components or a reply reference.
type SendData struct {
	Content           string             `json:"content,omitzero"`
	TTS               bool               `json:"tts,omitzero"`
	Embeds            []Embed            `json:"embeds,omitzero"`
	Components        []Component        `json:"components,omitzero"`
	MessageRef        *MessageRef        `json:"message_reference,omitzero"`
	AllowedMentions   *AllowedMentions   `json:"allowed_mentions,omitzero"`
	Flags             MessageFlags       `json:"flags,omitzero"`
	StickerIDs        []Snowflake        `json:"sticker_ids,omitzero"`
	Poll              *Poll              `json:"poll,omitzero"`
	Nonce             any                `json:"nonce,omitzero"`
	EnforceNonce      bool               `json:"enforce_nonce,omitzero"`
	SharedClientTheme *SharedClientTheme `json:"shared_client_theme,omitzero"`
}

// ComponentsMessage creates a Components V2 message with Discord's required
// flag already set.
func ComponentsMessage(components ...Component) SendData {
	return SendData{Components: components, Flags: MessageFlagIsComponentsV2}
}

// AllowedMentions controls which mentions in a message actually ping people.
//
// Without it, a bot echoing user input can be made to ping @everyone. Passing
// an empty AllowedMentions{} suppresses every ping, which is the safe default
// for anything built from untrusted text.
type AllowedMentions struct {
	Parse       []string    `json:"parse"` // "roles", "users", "everyone"
	Roles       []Snowflake `json:"roles,omitzero"`
	Users       []Snowflake `json:"users,omitzero"`
	RepliedUser bool        `json:"replied_user,omitzero"`
}

// NoMentions is an AllowedMentions that lets nothing ping. Use it whenever a
// message includes text a user supplied.
func NoMentions() *AllowedMentions { return &AllowedMentions{Parse: []string{}} }

// Send posts a plain-text message to a channel.
func (c *Client) Send(ctx context.Context, channelID Snowflake, content string) (*Message, error) {
	return c.SendComplex(ctx, channelID, SendData{Content: content})
}

// SendComplex posts a message with embeds, components or a reply reference.
func (c *Client) SendComplex(ctx context.Context, channelID Snowflake, data SendData) (*Message, error) {
	var msg Message
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/messages",
		Route:  "POST /channels/" + channelID.String() + "/messages",
		Body:   data,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// Reply posts a message as a reply to another one.
func (c *Client) Reply(ctx context.Context, channelID, messageID Snowflake, content string) (*Message, error) {
	return c.SendComplex(ctx, channelID, SendData{
		Content:    content,
		MessageRef: &MessageRef{MessageID: messageID, ChannelID: channelID},
	})
}

// EditMessage replaces a message's content. A bot may only edit its own
// messages.
func (c *Client) EditMessage(ctx context.Context, channelID, messageID Snowflake, data SendData) (*Message, error) {
	var msg Message
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/channels/" + channelID.String() + "/messages/" + messageID.String(),
		Route:  "PATCH /channels/" + channelID.String() + "/messages/{id}",
		Body:   data,
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// DeleteMessage removes a message.
func (c *Client) DeleteMessage(ctx context.Context, channelID, messageID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + channelID.String() + "/messages/" + messageID.String(),
		Route:  "DELETE /channels/" + channelID.String() + "/messages/{id}",
		Reason: reason,
	}, nil)
}

// Message fetches one message by ID.
func (c *Client) Message(ctx context.Context, channelID, messageID Snowflake) (*Message, error) {
	var msg Message
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String() + "/messages/" + messageID.String(),
		Route:  "GET /channels/" + channelID.String() + "/messages/{id}",
	}, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// MessagesQuery narrows a channel history request. Leave the anchors zero for
// the most recent messages.
type MessagesQuery struct {
	Limit  int       // 1-100, default 50
	Before Snowflake // messages older than this ID
	After  Snowflake // messages newer than this ID
	Around Snowflake // messages either side of this ID
}

// Messages fetches a page of a channel's history, newest first.
func (c *Client) Messages(ctx context.Context, channelID Snowflake, q MessagesQuery) ([]Message, error) {
	v := url.Values{}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}
	if !q.Before.IsZero() {
		v.Set("before", q.Before.String())
	}
	if !q.After.IsZero() {
		v.Set("after", q.After.String())
	}
	if !q.Around.IsZero() {
		v.Set("around", q.Around.String())
	}

	path := "/channels/" + channelID.String() + "/messages"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}

	var msgs []Message
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   path,
		Route:  "GET /channels/" + channelID.String() + "/messages",
	}, &msgs)
	return msgs, err
}

// React adds a reaction to a message as the bot.
func (c *Client) React(ctx context.Context, channelID, messageID Snowflake, emoji Emoji) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path: "/channels/" + channelID.String() + "/messages/" + messageID.String() +
			"/reactions/" + url.PathEscape(emoji.APIFormat()) + "/@me",
		Route: ownReactionRoute(channelID),
	}, nil)
}

// Unreact removes the bot's own reaction from a message.
func (c *Client) Unreact(ctx context.Context, channelID, messageID Snowflake, emoji Emoji) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path: "/channels/" + channelID.String() + "/messages/" + messageID.String() +
			"/reactions/" + url.PathEscape(emoji.APIFormat()) + "/@me",
		Route: ownReactionRoute(channelID),
	}, nil)
}

// Discord assigns adding and removing the current user's reaction to the same
// rate-limit bucket. Keep one local key so a known exhausted add also delays a
// remove instead of learning the shared limit through an avoidable 429.
func ownReactionRoute(channelID Snowflake) string {
	return "REACTION /channels/" + channelID.String() + "/messages/{id}/reactions/{emoji}/@me"
}

// Typing shows the "Bot is typing..." indicator for about ten seconds. Useful
// before a slow reply.
func (c *Client) Typing(ctx context.Context, channelID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/typing",
		Route:  "POST /channels/" + channelID.String() + "/typing",
	}, nil)
}

// Channel fetches a channel by ID.
func (c *Client) Channel(ctx context.Context, channelID Snowflake) (*Channel, error) {
	var ch Channel
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String(),
		Route:  "GET /channels/" + channelID.String(),
	}, &ch)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// Guild fetches a guild by ID.
func (c *Client) Guild(ctx context.Context, guildID Snowflake) (*Guild, error) {
	var g Guild
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String(),
		Route:  "GET /guilds/" + guildID.String(),
	}, &g)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// GuildMember fetches one member of a guild.
func (c *Client) GuildMember(ctx context.Context, guildID, userID Snowflake) (*Member, error) {
	var m Member
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/members/" + userID.String(),
		Route:  "GET /guilds/" + guildID.String() + "/members/{id}",
	}, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// User fetches a user by ID.
func (c *Client) User(ctx context.Context, userID Snowflake) (*User, error) {
	var u User
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/users/" + userID.String(),
		Route:  "GET /users/{id}",
	}, &u)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Event shortcuts

// Reply answers the message that triggered the event, as a Discord reply.
//
// It uses the background context; for control over cancellation use
// Client.Reply directly.
func (m *MessageCreate) Reply(content string) (*Message, error) {
	return m.c.Reply(context.Background(), m.ChannelID, m.ID, content)
}

// ReplyComplex answers with embeds or components. The reply reference is
// filled in for you if you leave it unset.
func (m *MessageCreate) ReplyComplex(data SendData) (*Message, error) {
	if data.MessageRef == nil {
		data.MessageRef = &MessageRef{MessageID: m.ID, ChannelID: m.ChannelID}
	}
	return m.c.SendComplex(context.Background(), m.ChannelID, data)
}

// Send posts to the same channel without marking it as a reply.
func (m *MessageCreate) Send(content string) (*Message, error) {
	return m.c.Send(context.Background(), m.ChannelID, content)
}

// React adds a reaction to the message.
func (m *MessageCreate) React(emoji string) error {
	return m.c.React(context.Background(), m.ChannelID, m.ID, Emoji{Name: emoji})
}

// Delete removes the message.
func (m *MessageCreate) Delete() error {
	return m.c.DeleteMessage(context.Background(), m.ChannelID, m.ID, "")
}

// IsFromBot reports whether the message was posted by any bot, including this
// one. Guarding handlers with it is the standard way to avoid loops.
func (m *MessageCreate) IsFromBot() bool { return m.Author != nil && m.Author.Bot }

// IsFromSelf reports whether this bot posted the message. Almost every
// message handler should start by returning early on it.
func (m *MessageCreate) IsFromSelf() bool {
	self := m.c.Self()
	return self != nil && m.Author != nil && m.Author.ID == self.ID
}
