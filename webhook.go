package starlings

import (
	"context"
	"net/http"
	"net/url"
)

// WebhookType distinguishes the kinds of webhook Discord issues.
type WebhookType int

const (
	// WebhookIncoming posts messages into a channel via a token URL.
	WebhookIncoming WebhookType = iota + 1
	// WebhookChannelFollower is created by following an announcement channel.
	WebhookChannelFollower
	// WebhookApplication backs interaction responses and follow-ups.
	WebhookApplication
)

// Webhook posts into a channel without a bot user. Its token is as sensitive
// as a bot token: anyone holding it can post as the webhook.
type Webhook struct {
	ID            Snowflake   `json:"id"`
	Type          WebhookType `json:"type"`
	GuildID       Snowflake   `json:"guild_id"`
	ChannelID     Snowflake   `json:"channel_id"`
	User          *User       `json:"user"` // who created it
	Name          string      `json:"name"`
	Avatar        string      `json:"avatar"`
	Token         string      `json:"token"`
	ApplicationID Snowflake   `json:"application_id"`
	URL           string      `json:"url"`
}

// WebhookParams are the settable fields of a webhook.
type WebhookParams struct {
	Name      *string    `json:"name,omitzero"`
	Avatar    *string    `json:"avatar,omitzero"` // image data URI
	ChannelID *Snowflake `json:"channel_id,omitzero"`
}

// CreateWebhook adds a webhook to a channel.
func (c *Client) CreateWebhook(ctx context.Context, channelID Snowflake, name, reason string) (*Webhook, error) {
	var out Webhook
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/webhooks",
		Route:  "POST /channels/" + channelID.String() + "/webhooks",
		Body:   map[string]string{"name": name},
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ChannelWebhooks lists a channel's webhooks.
func (c *Client) ChannelWebhooks(ctx context.Context, channelID Snowflake) ([]Webhook, error) {
	var out []Webhook
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String() + "/webhooks",
		Route:  "GET /channels/" + channelID.String() + "/webhooks",
	}, &out)
	return out, err
}

// GuildWebhooks lists every webhook in a guild.
func (c *Client) GuildWebhooks(ctx context.Context, guildID Snowflake) ([]Webhook, error) {
	var out []Webhook
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/webhooks",
		Route:  "GET /guilds/" + guildID.String() + "/webhooks",
	}, &out)
	return out, err
}

// Webhook fetches one webhook by ID, using the bot token.
func (c *Client) Webhook(ctx context.Context, webhookID Snowflake) (*Webhook, error) {
	var out Webhook
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/webhooks/" + webhookID.String(),
		Route:  "GET /webhooks/" + webhookID.String(),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// WebhookWithToken fetches a webhook using its token instead of the bot token,
// which needs no permissions at all.
func (c *Client) WebhookWithToken(ctx context.Context, webhookID Snowflake, token string) (*Webhook, error) {
	var out Webhook
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token),
		Route:  "GET /webhooks/" + webhookID.String() + "/{token}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyWebhook edits a webhook.
func (c *Client) ModifyWebhook(ctx context.Context, webhookID Snowflake, params WebhookParams, reason string) (*Webhook, error) {
	var out Webhook
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/webhooks/" + webhookID.String(),
		Route:  "PATCH /webhooks/" + webhookID.String(),
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteWebhook removes a webhook.
func (c *Client) DeleteWebhook(ctx context.Context, webhookID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/webhooks/" + webhookID.String(),
		Route:  "DELETE /webhooks/" + webhookID.String(),
		Reason: reason,
	}, nil)
}

// DeleteWebhookWithToken removes a webhook using its own token.
func (c *Client) DeleteWebhookWithToken(ctx context.Context, webhookID Snowflake, token string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token),
		Route:  "DELETE /webhooks/" + webhookID.String() + "/{token}",
	}, nil)
}

// WebhookMessage is what a webhook posts. Username and AvatarURL override the
// webhook's configured identity for this message only.
type WebhookMessage struct {
	Content         string           `json:"content,omitzero"`
	Username        string           `json:"username,omitzero"`
	AvatarURL       string           `json:"avatar_url,omitzero"`
	TTS             bool             `json:"tts,omitzero"`
	Embeds          []Embed          `json:"embeds,omitzero"`
	Components      []Component      `json:"components,omitzero"`
	AllowedMentions *AllowedMentions `json:"allowed_mentions,omitzero"`
	Flags           MessageFlags     `json:"flags,omitzero"`
	ThreadName      string           `json:"thread_name,omitzero"`
	Poll            *Poll            `json:"poll,omitzero"`
	AppliedTags     []Snowflake      `json:"applied_tags,omitzero"`
}

// ExecuteWebhook posts a message through a webhook.
//
// By default Discord returns no body; pass wait to have it return the created
// message instead, which is the only way to learn its ID.
func (c *Client) ExecuteWebhook(ctx context.Context, webhookID Snowflake, token string, msg WebhookMessage, wait bool, threadID Snowflake) (*Message, error) {
	v := url.Values{}
	if wait {
		v.Set("wait", "true")
	}
	if !threadID.IsZero() {
		v.Set("thread_id", threadID.String())
	}

	req := request{
		Method: http.MethodPost,
		Path:   "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) + query(v),
		Route:  "POST /webhooks/" + webhookID.String() + "/{token}",
		Body:   msg,
	}
	if !wait {
		return nil, c.rest.do(ctx, req, nil)
	}

	var out Message
	if err := c.rest.do(ctx, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WebhookMessageByID fetches a message a webhook posted.
func (c *Client) WebhookMessageByID(ctx context.Context, webhookID Snowflake, token string, messageID Snowflake) (*Message, error) {
	var out Message
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) +
			"/messages/" + messageID.String(),
		Route: "GET /webhooks/" + webhookID.String() + "/{token}/messages/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// EditWebhookMessage rewrites a message a webhook posted.
func (c *Client) EditWebhookMessage(ctx context.Context, webhookID Snowflake, token string, messageID Snowflake, msg WebhookMessage) (*Message, error) {
	var out Message
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path: "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) +
			"/messages/" + messageID.String(),
		Route: "PATCH /webhooks/" + webhookID.String() + "/{token}/messages/{id}",
		Body:  msg,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteWebhookMessage removes a message a webhook posted.
func (c *Client) DeleteWebhookMessage(ctx context.Context, webhookID Snowflake, token string, messageID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path: "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) +
			"/messages/" + messageID.String(),
		Route: "DELETE /webhooks/" + webhookID.String() + "/{token}/messages/{id}",
	}, nil)
}
