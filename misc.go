package starlings

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// GatewayInfo is what Discord recommends for connecting: the websocket URL,
// how many shards to use, and how many sessions are left today.
type GatewayInfo struct {
	URL               string `json:"url"`
	Shards            int    `json:"shards"`
	SessionStartLimit struct {
		Total          int `json:"total"`
		Remaining      int `json:"remaining"`
		ResetAfter     int `json:"reset_after"`
		MaxConcurrency int `json:"max_concurrency"`
	} `json:"session_start_limit"`
}

// GatewayBot returns the connection details for this bot, including the
// recommended shard count.
//
// starlings calls this itself when connecting; it is exported so a bot can
// check its remaining session budget before restarting.
func (c *Client) GatewayBot(ctx context.Context) (*GatewayInfo, error) {
	var out GatewayInfo
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/gateway/bot",
		Route:  "GET /gateway/bot",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Gateway returns the public websocket URL, with no authentication and no
// shard recommendation.
func (c *Client) Gateway(ctx context.Context) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/gateway",
		Route:  "GET /gateway",
	}, &out)
	return out.URL, err
}

// AddMember adds a user to a guild using an OAuth2 access token that carries
// the guilds.join scope.
func (c *Client) AddMember(ctx context.Context, guildID, userID Snowflake, accessToken string, nick string, roles []Snowflake, mute, deaf bool) (*Member, error) {
	body := map[string]any{"access_token": accessToken}
	if nick != "" {
		body["nick"] = nick
	}
	if len(roles) > 0 {
		body["roles"] = roles
	}
	if mute {
		body["mute"] = true
	}
	if deaf {
		body["deaf"] = true
	}

	var out Member
	err := c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/guilds/" + guildID.String() + "/members/" + userID.String(),
		Route:  "PUT /guilds/" + guildID.String() + "/members/{id}",
		Body:   body,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateSticker uploads a sticker to a guild.
//
// Discord takes stickers as multipart rather than a data URI, so the file
// travels as an attachment alongside the metadata.
func (c *Client) CreateSticker(ctx context.Context, guildID Snowflake, name, description, tags string, file File, reason string) (*Sticker, error) {
	var out Sticker
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/stickers",
		Route:  "POST /guilds/" + guildID.String() + "/stickers",
		Body: map[string]any{
			"name":        name,
			"description": description,
			"tags":        tags,
		},
		Files:  []File{file},
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SearchGuildMessages searches a guild's message history.
//
// Search is not available to most bots - Discord gates it - so expect a 403
// unless the application has been granted access.
func (c *Client) SearchGuildMessages(ctx context.Context, guildID Snowflake, params url.Values) (*MessageSearchResult, error) {
	var out MessageSearchResult
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/messages/search" + query(params),
		Route:  "GET /guilds/" + guildID.String() + "/messages/search",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// MessageSearchResult is a page of search hits. Messages is a list of groups
// because Discord returns each hit with its surrounding context.
type MessageSearchResult struct {
	TotalResults int         `json:"total_results"`
	Messages     [][]Message `json:"messages"`
}

// ThreadSearchResult is a page of forum or media channel threads.
type ThreadSearchResult struct {
	Threads      []Channel      `json:"threads"`
	Members      []ThreadMember `json:"members"`
	HasMore      bool           `json:"has_more"`
	TotalResults int            `json:"total_results"`
}

// SearchThreads searches a forum or media channel's posts.
func (c *Client) SearchThreads(ctx context.Context, channelID Snowflake, params url.Values) (*ThreadSearchResult, error) {
	var out ThreadSearchResult
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String() + "/threads/search" + query(params),
		Route:  "GET /channels/" + channelID.String() + "/threads/search",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GuildWidgetJSON is the public widget payload, readable without a token when
// the widget is enabled.
type GuildWidgetJSON struct {
	ID            Snowflake `json:"id"`
	Name          string    `json:"name"`
	InstantInvite string    `json:"instant_invite"`
	Channels      []Channel `json:"channels"`
	Members       []User    `json:"members"`
	PresenceCount int       `json:"presence_count"`
}

// GuildWidgetJSON reads a guild's public widget.
func (c *Client) GuildWidgetJSON(ctx context.Context, guildID Snowflake) (*GuildWidgetJSON, error) {
	var out GuildWidgetJSON
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/widget.json",
		Route:  "GET /guilds/" + guildID.String() + "/widget.json",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GuildWidgetImageURL returns the URL of a guild's widget PNG banner.
//
// style is one of "shield", "banner1", "banner2", "banner3" or "banner4".
// The image needs no authentication, so this returns a URL rather than fetching
// it - put it straight in an <img> tag or an embed.
func (c *Client) GuildWidgetImageURL(guildID Snowflake, style string) string {
	v := url.Values{}
	if style != "" {
		v.Set("style", style)
	}
	return BaseURL + "/guilds/" + guildID.String() + "/widget.png" + query(v)
}

// GuildWidgetImage fetches the widget PNG through the client, which is only
// necessary when the guild's widget is not public.
func (c *Client) GuildWidgetImage(ctx context.Context, guildID Snowflake, style string) error {
	v := url.Values{}
	if style != "" {
		v.Set("style", style)
	}
	return c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/widget.png" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/widget.png",
	}, nil)
}

// ExecuteSlackWebhook posts a Slack-formatted payload through a Discord
// webhook, which Discord translates. Useful for pointing existing Slack
// integrations at Discord unchanged.
func (c *Client) ExecuteSlackWebhook(ctx context.Context, webhookID Snowflake, token string, payload any, wait bool) error {
	v := url.Values{}
	if wait {
		v.Set("wait", "true")
	}
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) + "/slack" + query(v),
		Route:  "POST /webhooks/{id}/{token}/slack",
		Body:   payload,
	}, nil)
}

// ExecuteGitHubWebhook posts a GitHub webhook payload through a Discord
// webhook, giving the same rendering GitHub's own integration produces.
func (c *Client) ExecuteGitHubWebhook(ctx context.Context, webhookID Snowflake, token string, payload any, wait bool) error {
	v := url.Values{}
	if wait {
		v.Set("wait", "true")
	}
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) + "/github" + query(v),
		Route:  "POST /webhooks/{id}/{token}/github",
		Body:   payload,
	}, nil)
}

// ModifyWebhookWithToken edits a webhook using its own token rather than the
// bot token. The channel cannot be changed this way.
func (c *Client) ModifyWebhookWithToken(ctx context.Context, webhookID Snowflake, token, name, avatar string) (*Webhook, error) {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if avatar != "" {
		body["avatar"] = avatar
	}

	var out Webhook
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token),
		Route:  "PATCH /webhooks/{id}/{token}",
		Body:   body,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// OriginalWebhookMessage reads the first message a webhook posted, which for
// an interaction webhook is the original response.
func (c *Client) OriginalWebhookMessage(ctx context.Context, webhookID Snowflake, token string) (*Message, error) {
	var out Message
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/webhooks/" + webhookID.String() + "/" + url.PathEscape(token) + "/messages/@original",
		Route:  "GET /webhooks/{id}/{token}/messages/@original",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ScheduledEventException is a single occurrence of a recurring event that has
// been moved or cancelled.
type ScheduledEventException struct {
	EventExceptionID   Snowflake  `json:"event_exception_id"`
	EventID            Snowflake  `json:"event_id"`
	GuildID            Snowflake  `json:"guild_id"`
	IsCanceled         bool       `json:"is_canceled"`
	ScheduledStartTime *time.Time `json:"scheduled_start_time,omitzero"`
	ScheduledEndTime   *time.Time `json:"scheduled_end_time,omitzero"`
}

// CreateScheduledEventException moves or cancels one occurrence of a
// recurring event without touching the rest of the series.
func (c *Client) CreateScheduledEventException(ctx context.Context, guildID, eventID Snowflake, e ScheduledEventException) (*ScheduledEventException, error) {
	var out ScheduledEventException
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path: "/guilds/" + guildID.String() + "/scheduled-events/" +
			eventID.String() + "/exceptions",
		Route: "POST /guilds/" + guildID.String() + "/scheduled-events/{id}/exceptions",
		Body:  e,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyScheduledEventException edits an existing exception.
func (c *Client) ModifyScheduledEventException(ctx context.Context, guildID, eventID, exceptionID Snowflake, e ScheduledEventException) (*ScheduledEventException, error) {
	var out ScheduledEventException
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path: "/guilds/" + guildID.String() + "/scheduled-events/" +
			eventID.String() + "/exceptions/" + exceptionID.String(),
		Route: "PATCH /guilds/" + guildID.String() + "/scheduled-events/{id}/exceptions/{id}",
		Body:  e,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteScheduledEventException restores one occurrence of a recurring event.
func (c *Client) DeleteScheduledEventException(ctx context.Context, guildID, eventID, exceptionID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path: "/guilds/" + guildID.String() + "/scheduled-events/" +
			eventID.String() + "/exceptions/" + exceptionID.String(),
		Route: "DELETE /guilds/" + guildID.String() + "/scheduled-events/{id}/exceptions/{id}",
	}, nil)
}

// ScheduledEventUserCounts returns how many people are interested in an event.
func (c *Client) ScheduledEventUserCounts(ctx context.Context, guildID, eventID Snowflake) (map[string]int, error) {
	out := map[string]int{}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/guilds/" + guildID.String() + "/scheduled-events/" +
			eventID.String() + "/users/counts",
		Route: "GET /guilds/" + guildID.String() + "/scheduled-events/{id}/users/counts",
	}, &out)
	return out, err
}

// ScheduledEventExceptionUsers lists who is interested in one occurrence of a
// recurring event.
func (c *Client) ScheduledEventExceptionUsers(ctx context.Context, guildID, eventID, exceptionID Snowflake, params url.Values) ([]ScheduledEventUser, error) {
	var out []ScheduledEventUser
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/guilds/" + guildID.String() + "/scheduled-events/" + eventID.String() +
			"/" + exceptionID.String() + "/users" + query(params),
		Route: "GET /guilds/" + guildID.String() + "/scheduled-events/{id}/{id}/users",
	}, &out)
	return out, err
}

// OAuth2Keys returns Discord's JWKS, used to verify the identity tokens the
// Social SDK issues.
func (c *Client) OAuth2Keys(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/oauth2/keys",
		Route:  "GET /oauth2/keys",
	}, &out)
	return out, err
}

// OpenIDUserInfo is the OpenID Connect profile for a bearer token carrying the
// openid scope.
type OpenIDUserInfo struct {
	Sub      string `json:"sub"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email,omitzero"`
	Verified bool   `json:"email_verified,omitzero"`
}

// UserInfo reads the OpenID Connect profile for a bearer token.
func (c *Client) UserInfo(ctx context.Context, bearerToken string) (*OpenIDUserInfo, error) {
	var out OpenIDUserInfo
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/oauth2/userinfo",
		Route:  "GET /oauth2/userinfo",
		Auth:   BearerToken(bearerToken),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
