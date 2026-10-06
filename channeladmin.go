package starlings

import (
	"context"
	"net/http"
	"net/url"
)

// ChannelParams are the settable fields of a channel. All are optional: on
// create, omitted fields take Discord's defaults; on edit they are left alone.
type ChannelParams struct {
	Name                 *string      `json:"name,omitzero"`
	Type                 *ChannelType `json:"type,omitzero"`
	Topic                *string      `json:"topic,omitzero"`
	Position             *int         `json:"position,omitzero"`
	NSFW                 *bool        `json:"nsfw,omitzero"`
	RateLimitPerUser     *int         `json:"rate_limit_per_user,omitzero"` // slow mode, seconds
	Bitrate              *int         `json:"bitrate,omitzero"`             // voice
	UserLimit            *int         `json:"user_limit,omitzero"`          // voice
	ParentID             *Snowflake   `json:"parent_id,omitzero"`           // category
	PermissionOverwrites *[]Overwrite `json:"permission_overwrites,omitzero"`
	DefaultAutoArchive   *int         `json:"default_auto_archive_duration,omitzero"`
}

// GuildChannels lists every channel in a guild, excluding threads.
func (c *Client) GuildChannels(ctx context.Context, guildID Snowflake) ([]Channel, error) {
	var out []Channel
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/channels",
		Route:  "GET /guilds/" + guildID.String() + "/channels",
	}, &out)
	return out, err
}

// CreateChannel adds a channel to a guild.
func (c *Client) CreateChannel(ctx context.Context, guildID Snowflake, params ChannelParams, reason string) (*Channel, error) {
	var out Channel
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/channels",
		Route:  "POST /guilds/" + guildID.String() + "/channels",
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyChannel edits a channel.
func (c *Client) ModifyChannel(ctx context.Context, channelID Snowflake, params ChannelParams, reason string) (*Channel, error) {
	var out Channel
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/channels/" + channelID.String(),
		Route:  "PATCH /channels/" + channelID.String(),
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteChannel removes a channel, or closes a DM. Deleting a category does
// not delete the channels inside it; they simply lose their parent.
func (c *Client) DeleteChannel(ctx context.Context, channelID Snowflake, reason string) (*Channel, error) {
	var out Channel
	err := c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + channelID.String(),
		Route:  "DELETE /channels/" + channelID.String(),
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ChannelPosition is one entry of a channel reordering request.
type ChannelPosition struct {
	ID              Snowflake  `json:"id"`
	Position        *int       `json:"position,omitzero"`
	LockPermissions *bool      `json:"lock_permissions,omitzero"`
	ParentID        *Snowflake `json:"parent_id,omitzero"`
}

// ReorderChannels moves channels within a guild.
func (c *Client) ReorderChannels(ctx context.Context, guildID Snowflake, positions []ChannelPosition, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/channels",
		Route:  "PATCH /guilds/" + guildID.String() + "/channels",
		Body:   positions,
		Reason: reason,
	}, nil)
}

// Permissions

// OverwriteType distinguishes a role overwrite from a member one.
const (
	OverwriteRole   = 0
	OverwriteMember = 1
)

// SetPermissionOverwrite adds or replaces a channel permission overwrite for
// one role or member.
//
// Deny is applied before Allow. Permissions left out of both are inherited
// from the category or the guild.
func (c *Client) SetPermissionOverwrite(ctx context.Context, channelID, overwriteID Snowflake, allow, deny Permissions, kind int, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/channels/" + channelID.String() + "/permissions/" + overwriteID.String(),
		Route:  "PUT /channels/" + channelID.String() + "/permissions/{id}",
		Body: map[string]any{
			"allow": allow.String(),
			"deny":  deny.String(),
			"type":  kind,
		},
		Reason: reason,
	}, nil)
}

// DeletePermissionOverwrite removes an overwrite, restoring inheritance.
func (c *Client) DeletePermissionOverwrite(ctx context.Context, channelID, overwriteID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + channelID.String() + "/permissions/" + overwriteID.String(),
		Route:  "DELETE /channels/" + channelID.String() + "/permissions/{id}",
		Reason: reason,
	}, nil)
}

// Pins

// Pins lists a channel's pinned messages, most recently pinned first.
func (c *Client) Pins(ctx context.Context, channelID Snowflake) ([]Message, error) {
	var out []Message
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String() + "/pins",
		Route:  "GET /channels/" + channelID.String() + "/pins",
	}, &out)
	return out, err
}

// PinMessage pins a message. A channel holds at most 50 pins.
func (c *Client) PinMessage(ctx context.Context, channelID, messageID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/channels/" + channelID.String() + "/pins/" + messageID.String(),
		Route:  "PUT /channels/" + channelID.String() + "/pins/{id}",
		Reason: reason,
	}, nil)
}

// UnpinMessage removes a pin.
func (c *Client) UnpinMessage(ctx context.Context, channelID, messageID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + channelID.String() + "/pins/" + messageID.String(),
		Route:  "DELETE /channels/" + channelID.String() + "/pins/{id}",
		Reason: reason,
	}, nil)
}

// Invites

// Invite is a link that lets people join a guild.
type Invite struct {
	Code                     string    `json:"code"`
	Guild                    *Guild    `json:"guild"`
	Channel                  *Channel  `json:"channel"`
	Inviter                  *User     `json:"inviter"`
	TargetType               int       `json:"target_type"`
	TargetUser               *User     `json:"target_user"`
	ApproximatePresenceCount int       `json:"approximate_presence_count"`
	ApproximateMemberCount   int       `json:"approximate_member_count"`
	ExpiresAt                *string   `json:"expires_at"`
	Uses                     int       `json:"uses"`
	MaxUses                  int       `json:"max_uses"`
	MaxAge                   int       `json:"max_age"`
	Temporary                bool      `json:"temporary"`
	CreatedAt                string    `json:"created_at"`
	GuildID                  Snowflake `json:"guild_id"`
}

// InviteParams configures a new invite. Zero values mean Discord's defaults:
// 24 hours, unlimited uses.
type InviteParams struct {
	MaxAge    *int  `json:"max_age,omitzero"`  // seconds, 0 = never expires
	MaxUses   *int  `json:"max_uses,omitzero"` // 0 = unlimited
	Temporary *bool `json:"temporary,omitzero"`
	Unique    *bool `json:"unique,omitzero"` // never reuse a similar existing invite
}

// ChannelInvites lists the invites pointing at a channel.
func (c *Client) ChannelInvites(ctx context.Context, channelID Snowflake) ([]Invite, error) {
	var out []Invite
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String() + "/invites",
		Route:  "GET /channels/" + channelID.String() + "/invites",
	}, &out)
	return out, err
}

// CreateInvite makes a new invite to a channel.
func (c *Client) CreateInvite(ctx context.Context, channelID Snowflake, params InviteParams, reason string) (*Invite, error) {
	var out Invite
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/invites",
		Route:  "POST /channels/" + channelID.String() + "/invites",
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GuildInvites lists every invite in a guild.
func (c *Client) GuildInvites(ctx context.Context, guildID Snowflake) ([]Invite, error) {
	var out []Invite
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/invites",
		Route:  "GET /guilds/" + guildID.String() + "/invites",
	}, &out)
	return out, err
}

// Invite looks up an invite by its code. Counts are only included when
// withCounts is set.
func (c *Client) Invite(ctx context.Context, code string, withCounts bool) (*Invite, error) {
	v := url.Values{}
	if withCounts {
		v.Set("with_counts", "true")
	}

	var out Invite
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/invites/" + url.PathEscape(code) + query(v),
		Route:  "GET /invites/{code}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteInvite revokes an invite.
func (c *Client) DeleteInvite(ctx context.Context, code, reason string) (*Invite, error) {
	var out Invite
	err := c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/invites/" + url.PathEscape(code),
		Route:  "DELETE /invites/{code}",
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// FollowChannel makes an announcement channel's posts crosspost into target.
func (c *Client) FollowChannel(ctx context.Context, channelID, targetID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/followers",
		Route:  "POST /channels/" + channelID.String() + "/followers",
		Body:   map[string]any{"webhook_channel_id": targetID},
	}, nil)
}
