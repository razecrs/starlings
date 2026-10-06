package starlings

import (
	"context"
	"net/http"
	"net/url"
)

// Connection is a third-party account a user has linked to Discord.
type Connection struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"` // "github", "spotify", "steam", ...
	Revoked      bool   `json:"revoked"`
	Verified     bool   `json:"verified"`
	FriendSync   bool   `json:"friend_sync"`
	ShowActivity bool   `json:"show_activity"`
	Visibility   int    `json:"visibility"`
}

// CurrentUser fetches the account the token belongs to - for a bot token,
// the bot's own user.
func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	var out User
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/users/@me",
		Route:  "GET /users/@me",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyCurrentUser changes the bot's username or avatar. Leave a field empty
// to keep it; avatar is a data URI.
func (c *Client) ModifyCurrentUser(ctx context.Context, username, avatar string) (*User, error) {
	body := map[string]any{}
	if username != "" {
		body["username"] = username
	}
	if avatar != "" {
		body["avatar"] = avatar
	}

	var out User
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/users/@me",
		Route:  "PATCH /users/@me",
		Body:   body,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CurrentUserGuildsQuery pages through the guilds a bot is in.
type CurrentUserGuildsQuery struct {
	Before     Snowflake
	After      Snowflake
	Limit      int // 1-200
	WithCounts bool
}

// CurrentUserGuilds lists the guilds the bot is a member of.
//
// The Guild objects are partial - enough for ID, name, icon and permissions,
// but not channels or members.
func (c *Client) CurrentUserGuilds(ctx context.Context, q CurrentUserGuildsQuery) ([]Guild, error) {
	v := url.Values{}
	if !q.Before.IsZero() {
		v.Set("before", q.Before.String())
	}
	if !q.After.IsZero() {
		v.Set("after", q.After.String())
	}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}
	if q.WithCounts {
		v.Set("with_counts", "true")
	}

	var out []Guild
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/users/@me/guilds" + query(v),
		Route:  "GET /users/@me/guilds",
	}, &out)
	return out, err
}

// CurrentUserGuildMember fetches the bot's own member object in a guild,
// which is the cheapest way to read its own roles and permissions there.
func (c *Client) CurrentUserGuildMember(ctx context.Context, guildID Snowflake) (*Member, error) {
	var out Member
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/users/@me/guilds/" + guildID.String() + "/member",
		Route:  "GET /users/@me/guilds/{id}/member",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// LeaveGuild removes the bot from a guild.
func (c *Client) LeaveGuild(ctx context.Context, guildID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/users/@me/guilds/" + guildID.String(),
		Route:  "DELETE /users/@me/guilds/{id}",
	}, nil)
}

// UserConnections lists the current account's linked third-party accounts.
// Needs an OAuth2 token with the connections scope; a bot token returns an
// empty list.
func (c *Client) UserConnections(ctx context.Context) ([]Connection, error) {
	var out []Connection
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/users/@me/connections",
		Route:  "GET /users/@me/connections",
	}, &out)
	return out, err
}

// CreateDM opens a direct-message channel with a user.
//
// Discord caches these, so calling it repeatedly for the same user returns the
// same channel rather than creating duplicates. Note that a DM can still fail
// to send if the recipient shares no guild with the bot or blocks DMs.
func (c *Client) CreateDM(ctx context.Context, userID Snowflake) (*Channel, error) {
	var out Channel
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/users/@me/channels",
		Route:  "POST /users/@me/channels",
		Body:   map[string]any{"recipient_id": userID},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SendDM opens a DM channel with a user and posts a message in one step.
func (c *Client) SendDM(ctx context.Context, userID Snowflake, content string) (*Message, error) {
	ch, err := c.CreateDM(ctx, userID)
	if err != nil {
		return nil, err
	}
	return c.Send(ctx, ch.ID, content)
}

// AddGroupDMRecipient adds a user to a group DM. Needs an OAuth2 token with
// the gdm.join scope for that user.
func (c *Client) AddGroupDMRecipient(ctx context.Context, channelID, userID Snowflake, accessToken, nick string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/channels/" + channelID.String() + "/recipients/" + userID.String(),
		Route:  "PUT /channels/" + channelID.String() + "/recipients/{id}",
		Body:   map[string]any{"access_token": accessToken, "nick": nick},
	}, nil)
}

// RemoveGroupDMRecipient removes a user from a group DM.
func (c *Client) RemoveGroupDMRecipient(ctx context.Context, channelID, userID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + channelID.String() + "/recipients/" + userID.String(),
		Route:  "DELETE /channels/" + channelID.String() + "/recipients/{id}",
	}, nil)
}
