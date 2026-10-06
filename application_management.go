package starlings

import (
	"context"
	"net/http"
)

// CommandPermissionType says what a permission override applies to.
const (
	CommandPermissionRole    = 1
	CommandPermissionUser    = 2
	CommandPermissionChannel = 3
)

// CommandPermission allows or denies one role, user or channel access to a
// command.
//
// The guild ID itself as a role ID means @everyone; the guild ID minus one as
// a channel ID means "all channels".
type CommandPermission struct {
	ID         Snowflake `json:"id"`
	Type       int       `json:"type"`
	Permission bool      `json:"permission"`
}

// GuildCommandPermissions is the override set for one command in one guild.
type GuildCommandPermissions struct {
	ID            Snowflake           `json:"id"`
	ApplicationID Snowflake           `json:"application_id"`
	GuildID       Snowflake           `json:"guild_id"`
	Permissions   []CommandPermission `json:"permissions"`
}

// ApplicationCommand fetches one global command.
func (c *Client) ApplicationCommand(ctx context.Context, appID, commandID Snowflake) (*ApplicationCommand, error) {
	var out ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/commands/" + commandID.String(),
		Route:  "GET /applications/{id}/commands/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// EditApplicationCommand updates one global command in place, which avoids the
// propagation delay of deleting and recreating it.
func (c *Client) EditApplicationCommand(ctx context.Context, appID, commandID Snowflake, cmd ApplicationCommand) (*ApplicationCommand, error) {
	var out ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/applications/" + appID.String() + "/commands/" + commandID.String(),
		Route:  "PATCH /applications/{id}/commands/{id}",
		Body:   cmd,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GuildCommand fetches one guild command.
func (c *Client) GuildCommand(ctx context.Context, appID, guildID, commandID Snowflake) (*ApplicationCommand, error) {
	var out ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/applications/" + appID.String() + "/guilds/" + guildID.String() +
			"/commands/" + commandID.String(),
		Route: "GET /applications/{id}/guilds/{id}/commands/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// EditGuildCommand updates one guild command.
func (c *Client) EditGuildCommand(ctx context.Context, appID, guildID, commandID Snowflake, cmd ApplicationCommand) (*ApplicationCommand, error) {
	var out ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path: "/applications/" + appID.String() + "/guilds/" + guildID.String() +
			"/commands/" + commandID.String(),
		Route: "PATCH /applications/{id}/guilds/{id}/commands/{id}",
		Body:  cmd,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GuildCommandsPermissions lists the permission overrides for every command
// the application has in a guild.
func (c *Client) GuildCommandsPermissions(ctx context.Context, appID, guildID Snowflake) ([]GuildCommandPermissions, error) {
	var out []GuildCommandPermissions
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/applications/" + appID.String() + "/guilds/" + guildID.String() +
			"/commands/permissions",
		Route: "GET /applications/{id}/guilds/{id}/commands/permissions",
	}, &out)
	return out, err
}

// GuildCommandPermissions reads one command's overrides in a guild.
func (c *Client) GuildCommandPermissions(ctx context.Context, appID, guildID, commandID Snowflake) (*GuildCommandPermissions, error) {
	var out GuildCommandPermissions
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/applications/" + appID.String() + "/guilds/" + guildID.String() +
			"/commands/" + commandID.String() + "/permissions",
		Route: "GET /applications/{id}/guilds/{id}/commands/{id}/permissions",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetGuildCommandPermissions replaces a command's overrides in a guild.
//
// This one is unusual: it needs a *bearer* token from a user who can manage
// roles in that guild, not the bot token - so pass one obtained through
// OAuth2 rather than relying on the client's own credentials.
func (c *Client) SetGuildCommandPermissions(ctx context.Context, bearerToken string, appID, guildID, commandID Snowflake, perms []CommandPermission) (*GuildCommandPermissions, error) {
	var out GuildCommandPermissions
	err := c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path: "/applications/" + appID.String() + "/guilds/" + guildID.String() +
			"/commands/" + commandID.String() + "/permissions",
		Route: "PUT /applications/{id}/guilds/{id}/commands/{id}/permissions",
		Auth:  BearerToken(bearerToken),
		Body:  map[string]any{"permissions": perms},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Application fetches any application by ID.
func (c *Client) Application(ctx context.Context, appID Snowflake) (*CurrentApplication, error) {
	var out CurrentApplication
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String(),
		Route:  "GET /applications/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplicationParams are the settable fields of an application.
type ApplicationParams struct {
	Description                    *string   `json:"description,omitzero"`
	Icon                           *string   `json:"icon,omitzero"` // data URI
	CoverImage                     *string   `json:"cover_image,omitzero"`
	Tags                           *[]string `json:"tags,omitzero"` // max 5
	InteractionsEndpointURL        *string   `json:"interactions_endpoint_url,omitzero"`
	RoleConnectionsVerificationURL *string   `json:"role_connections_verification_url,omitzero"`
	Flags                          *int      `json:"flags,omitzero"`
	CustomInstallURL               *string   `json:"custom_install_url,omitzero"`
	RedirectURIs                   *[]string `json:"redirect_uris,omitzero"`
}

// EditApplication updates the application the token belongs to.
func (c *Client) EditApplication(ctx context.Context, params ApplicationParams) (*CurrentApplication, error) {
	var out CurrentApplication
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/applications/@me",
		Route:  "PATCH /applications/@me",
		Body:   params,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// EditApplicationByID updates a specific application.
func (c *Client) EditApplicationByID(ctx context.Context, appID Snowflake, params ApplicationParams) (*CurrentApplication, error) {
	var out CurrentApplication
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/applications/" + appID.String(),
		Route:  "PATCH /applications/{id}",
		Body:   params,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// OAuth2Authorization describes the token used to make a request: which
// application it belongs to, what scopes it carries and when it expires.
type OAuth2Authorization struct {
	Application *CurrentApplication `json:"application"`
	Scopes      []string            `json:"scopes"`
	Expires     string              `json:"expires"`
	User        *User               `json:"user"`
}

// CurrentAuthorization describes the current OAuth2 token. Useful for
// checking which scopes a bearer token actually carries before relying on it.
func (c *Client) CurrentAuthorization(ctx context.Context, bearerToken string) (*OAuth2Authorization, error) {
	var out OAuth2Authorization
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/oauth2/@me",
		Route:  "GET /oauth2/@me",
		Auth:   BearerToken(bearerToken),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CurrentOAuth2Application returns the application behind the token, through
// the OAuth2 endpoint rather than the applications one.
func (c *Client) CurrentOAuth2Application(ctx context.Context) (*CurrentApplication, error) {
	var out CurrentApplication
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/oauth2/applications/@me",
		Route:  "GET /oauth2/applications/@me",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
