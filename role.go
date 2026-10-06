package starlings

import (
	"context"
	"net/http"
)

// RoleParams are the fields settable when creating or editing a role. All are
// optional; omitted ones take Discord's defaults on create, and are left alone
// on edit.
type RoleParams struct {
	Name         *string      `json:"name,omitzero"`
	Permissions  *Permissions `json:"permissions,string,omitzero"`
	Color        *int         `json:"color,omitzero"` // 0xRRGGBB, 0 means "no colour"
	Hoist        *bool        `json:"hoist,omitzero"` // list separately in the sidebar
	Icon         *string      `json:"icon,omitzero"`  // image data URI
	UnicodeEmoji *string      `json:"unicode_emoji,omitzero"`
	Mentionable  *bool        `json:"mentionable,omitzero"`
}

// Roles lists a guild's roles, including @everyone.
func (c *Client) Roles(ctx context.Context, guildID Snowflake) ([]Role, error) {
	var out []Role
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/roles",
		Route:  "GET /guilds/" + guildID.String() + "/roles",
	}, &out)
	return out, err
}

// Role fetches one role.
func (c *Client) Role(ctx context.Context, guildID, roleID Snowflake) (*Role, error) {
	var out Role
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/roles/" + roleID.String(),
		Route:  "GET /guilds/" + guildID.String() + "/roles/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateRole adds a role to a guild.
//
// A bot can only grant permissions it holds itself, and can only manage roles
// below its own highest role - both of which surface as a 403.
func (c *Client) CreateRole(ctx context.Context, guildID Snowflake, params RoleParams, reason string) (*Role, error) {
	var out Role
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/roles",
		Route:  "POST /guilds/" + guildID.String() + "/roles",
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyRole edits a role.
func (c *Client) ModifyRole(ctx context.Context, guildID, roleID Snowflake, params RoleParams, reason string) (*Role, error) {
	var out Role
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/roles/" + roleID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/roles/{id}",
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole removes a role from a guild.
func (c *Client) DeleteRole(ctx context.Context, guildID, roleID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/roles/" + roleID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/roles/{id}",
		Reason: reason,
	}, nil)
}

// RolePosition is one entry of a reordering request.
type RolePosition struct {
	ID       Snowflake `json:"id"`
	Position *int      `json:"position,omitzero"`
}

// ReorderRoles changes where roles sit in the hierarchy, which is what decides
// permission precedence as well as sidebar order.
func (c *Client) ReorderRoles(ctx context.Context, guildID Snowflake, positions []RolePosition, reason string) ([]Role, error) {
	var out []Role
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/roles",
		Route:  "PATCH /guilds/" + guildID.String() + "/roles",
		Body:   positions,
		Reason: reason,
	}, &out)
	return out, err
}

// RoleMemberCounts returns how many members hold each role, keyed by role ID.
func (c *Client) RoleMemberCounts(ctx context.Context, guildID Snowflake) (map[Snowflake]int, error) {
	out := map[Snowflake]int{}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/roles/member-counts",
		Route:  "GET /guilds/" + guildID.String() + "/roles/member-counts",
	}, &out)
	return out, err
}
