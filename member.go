package starlings

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// MembersQuery pages through a guild's member list. Discord returns members in
// ascending user-ID order, so paging means passing the last ID back as After.
type MembersQuery struct {
	Limit int       // 1-1000, default 1
	After Snowflake // members with a higher ID than this
}

// Members lists a guild's members.
//
// This needs the privileged GuildMembers intent. Without it Discord returns
// only the bot itself, which looks like an empty guild rather than an error.
func (c *Client) Members(ctx context.Context, guildID Snowflake, q MembersQuery) ([]Member, error) {
	v := url.Values{}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}
	if !q.After.IsZero() {
		v.Set("after", q.After.String())
	}

	var out []Member
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/members" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/members",
	}, &out)
	return out, err
}

// SearchMembers finds members whose username or nickname starts with the given
// text. Limit is 1-1000.
func (c *Client) SearchMembers(ctx context.Context, guildID Snowflake, name string, limit int) ([]Member, error) {
	v := url.Values{"query": {name}}
	if limit > 0 {
		v.Set("limit", itoa(limit))
	}

	var out []Member
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/members/search" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/members/search",
	}, &out)
	return out, err
}

// ModifyMember is the set of member fields a bot may change. Every field is
// optional: a nil pointer leaves that attribute alone, which is why these are
// pointers rather than plain values.
type ModifyMember struct {
	Nick  *string      `json:"nick,omitzero"`
	Roles *[]Snowflake `json:"roles,omitzero"`
	Mute  *bool        `json:"mute,omitzero"`
	Deaf  *bool        `json:"deaf,omitzero"`

	// ChannelID moves the member between voice channels. A pointer to zero
	// disconnects them.
	ChannelID *Snowflake `json:"channel_id,omitzero"`

	// CommunicationDisabledUntil times a member out, up to 28 days ahead.
	// A pointer to nil clears an existing timeout.
	CommunicationDisabledUntil **time.Time `json:"communication_disabled_until,omitzero"`

	Flags *int `json:"flags,omitzero"`
}

// ModifyMember changes a member's nickname, roles, voice state or timeout.
func (c *Client) ModifyMember(ctx context.Context, guildID, userID Snowflake, m ModifyMember, reason string) (*Member, error) {
	var out Member
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/members/" + userID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/members/{id}",
		Body:   m,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetNickname changes the bot's own nickname in a guild.
func (c *Client) SetNickname(ctx context.Context, guildID Snowflake, nick string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/members/@me",
		Route:  "PATCH /guilds/" + guildID.String() + "/members/@me",
		Body:   map[string]string{"nick": nick},
	}, nil)
}

// MaxTimeout is the longest timeout Discord accepts.
const MaxTimeout = 28 * 24 * time.Hour

// Timeout mutes a member until the given time, up to MaxTimeout ahead. Pass
// the zero time, or call ClearTimeout, to lift a timeout early.
func (c *Client) Timeout(ctx context.Context, guildID, userID Snowflake, until time.Time, reason string) (*Member, error) {
	if !until.IsZero() && time.Until(until) > MaxTimeout {
		return nil, fmt.Errorf("starlings: a timeout can last at most %d days", int(MaxTimeout/(24*time.Hour)))
	}
	var t *time.Time
	if !until.IsZero() {
		t = &until
	}
	return c.ModifyMember(ctx, guildID, userID, ModifyMember{
		CommunicationDisabledUntil: &t,
	}, reason)
}

// ClearTimeout lifts a member's timeout early.
func (c *Client) ClearTimeout(ctx context.Context, guildID, userID Snowflake, reason string) (*Member, error) {
	return c.Timeout(ctx, guildID, userID, time.Time{}, reason)
}

// AddMemberRole grants a role.
func (c *Client) AddMemberRole(ctx context.Context, guildID, userID, roleID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path: "/guilds/" + guildID.String() + "/members/" + userID.String() +
			"/roles/" + roleID.String(),
		Route:  "PUT /guilds/" + guildID.String() + "/members/{id}/roles/{id}",
		Reason: reason,
	}, nil)
}

// RemoveMemberRole takes a role away.
func (c *Client) RemoveMemberRole(ctx context.Context, guildID, userID, roleID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path: "/guilds/" + guildID.String() + "/members/" + userID.String() +
			"/roles/" + roleID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/members/{id}/roles/{id}",
		Reason: reason,
	}, nil)
}

// Kick removes a member from a guild. They can rejoin with a new invite.
func (c *Client) Kick(ctx context.Context, guildID, userID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/members/" + userID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/members/{id}",
		Reason: reason,
	}, nil)
}

// Bans

// Ban is a guild ban record.
type Ban struct {
	Reason string `json:"reason"`
	User   *User  `json:"user"`
}

// BansQuery pages through a guild's ban list.
type BansQuery struct {
	Limit  int
	Before Snowflake
	After  Snowflake
}

// Bans lists a guild's bans.
func (c *Client) Bans(ctx context.Context, guildID Snowflake, q BansQuery) ([]Ban, error) {
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

	var out []Ban
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/bans" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/bans",
	}, &out)
	return out, err
}

// Ban looks up one ban, returning a 404 error if the user is not banned.
func (c *Client) Ban(ctx context.Context, guildID, userID Snowflake) (*Ban, error) {
	var out Ban
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/bans/" + userID.String(),
		Route:  "GET /guilds/" + guildID.String() + "/bans/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateBan bans a user, optionally deleting their recent messages.
//
// deleteMessageSeconds may be up to 7 days (604800). The user need not be in
// the guild - banning an absent user pre-empts them joining.
func (c *Client) CreateBan(ctx context.Context, guildID, userID Snowflake, deleteMessageSeconds int, reason string) error {
	body := map[string]any{}
	if deleteMessageSeconds > 0 {
		body["delete_message_seconds"] = deleteMessageSeconds
	}
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/guilds/" + guildID.String() + "/bans/" + userID.String(),
		Route:  "PUT /guilds/" + guildID.String() + "/bans/{id}",
		Body:   body,
		Reason: reason,
	}, nil)
}

// RemoveBan unbans a user.
func (c *Client) RemoveBan(ctx context.Context, guildID, userID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/bans/" + userID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/bans/{id}",
		Reason: reason,
	}, nil)
}

// BulkBanResult reports which users a bulk ban actually affected.
type BulkBanResult struct {
	BannedUsers []Snowflake `json:"banned_users"`
	FailedUsers []Snowflake `json:"failed_users"`
}

// BulkBan bans up to 200 users in one call, which is far cheaper than a ban
// each when clearing a raid.
func (c *Client) BulkBan(ctx context.Context, guildID Snowflake, userIDs []Snowflake, deleteMessageSeconds int, reason string) (*BulkBanResult, error) {
	body := map[string]any{"user_ids": userIDs}
	if deleteMessageSeconds > 0 {
		body["delete_message_seconds"] = deleteMessageSeconds
	}

	var out BulkBanResult
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/bulk-ban",
		Route:  "POST /guilds/" + guildID.String() + "/bulk-ban",
		Body:   body,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Prune

// PruneCount reports how many members would be removed by a prune.
func (c *Client) PruneCount(ctx context.Context, guildID Snowflake, days int, includeRoles []Snowflake) (int, error) {
	v := url.Values{}
	if days > 0 {
		v.Set("days", itoa(days))
	}
	for _, r := range includeRoles {
		v.Add("include_roles", r.String())
	}

	var out struct {
		Pruned int `json:"pruned"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/prune" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/prune",
	}, &out)
	return out.Pruned, err
}

// Prune removes members with no roles who have been inactive for the given
// number of days.
//
// Set computeCount to false on a large guild: counting is expensive, and
// Discord recommends skipping it, in which case the returned count is zero.
func (c *Client) Prune(ctx context.Context, guildID Snowflake, days int, computeCount bool, includeRoles []Snowflake, reason string) (int, error) {
	body := map[string]any{"compute_prune_count": computeCount}
	if days > 0 {
		body["days"] = days
	}
	if len(includeRoles) > 0 {
		body["include_roles"] = includeRoles
	}

	var out struct {
		Pruned int `json:"pruned"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/prune",
		Route:  "POST /guilds/" + guildID.String() + "/prune",
		Body:   body,
		Reason: reason,
	}, &out)
	return out.Pruned, err
}

// query renders values as a URL query string, or "" when there are none.
func query(v url.Values) string {
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}
