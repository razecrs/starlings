package starlings

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// MetadataType is the comparison a guild applies to one role-connection
// metadata value when deciding whether to grant a linked role.
//
// The type matters for role rules, not for display: the profile card shows
// whatever value you push regardless of which comparison is declared.
type MetadataType int

const (
	MetadataIntegerLessThanOrEqual MetadataType = iota + 1
	MetadataIntegerGreaterThanOrEqual
	MetadataIntegerEqual
	MetadataIntegerNotEqual
	MetadataDateTimeLessThanOrEqual    // value is an ISO8601 string
	MetadataDateTimeGreaterThanOrEqual // value is an ISO8601 string
	MetadataBooleanEqual               // value is "1" or "0"
	MetadataBooleanNotEqual
)

// MaxRoleConnectionMetadata is the number of metadata records one application
// may register. Discord rejects anything above it.
const MaxRoleConnectionMetadata = 5

// ErrTooManyMetadataRecords is returned before the request is sent when more
// than MaxRoleConnectionMetadata records are supplied.
var ErrTooManyMetadataRecords = errors.New(
	"starlings: an application may register at most 5 role connection metadata records")

// RoleConnectionMetadata declares one field an application can publish about a
// user. Registering these is what makes the fields appear on the connection
// card on a user's profile, and what guilds build linked-role rules from.
//
// Key is the identifier used when pushing values; Name and Description are
// what people see.
type RoleConnectionMetadata struct {
	Key                      string            `json:"key"`  // a-z, 0-9 and _ only, max 50
	Name                     string            `json:"name"` // max 100, shown on the card
	NameLocalizations        map[string]string `json:"name_localizations,omitzero"`
	Description              string            `json:"description"` // max 200
	DescriptionLocalizations map[string]string `json:"description_localizations,omitzero"`
	Type                     MetadataType      `json:"type"`
}

// RoleConnection is the card shown on a user's profile for one application:
// a platform name, an optional username line, and the values for the metadata
// keys the application registered.
//
// Metadata values are always strings on the wire, even the numeric ones.
type RoleConnection struct {
	PlatformName     string            `json:"platform_name,omitzero"`     // the card's title, max 50
	PlatformUsername string            `json:"platform_username,omitzero"` // the line under it, max 100
	Metadata         map[string]string `json:"metadata,omitzero"`
}

// RoleConnectionMetadataRecords returns the metadata schema an application has
// registered. It authenticates with the bot token.
func (c *Client) RoleConnectionMetadataRecords(ctx context.Context, appID Snowflake) ([]RoleConnectionMetadata, error) {
	var out []RoleConnectionMetadata
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/role-connections/metadata",
		Route:  "GET /applications/{id}/role-connections/metadata",
	}, &out)
	return out, err
}

// RegisterRoleConnectionMetadata replaces an application's metadata schema.
// This is the first half of publishing a profile card: it declares which
// fields exist. It authenticates with the bot token.
//
// At most MaxRoleConnectionMetadata records are allowed, which is checked here
// so the failure is legible rather than a 400 from Discord.
func (c *Client) RegisterRoleConnectionMetadata(ctx context.Context, appID Snowflake, records []RoleConnectionMetadata) ([]RoleConnectionMetadata, error) {
	if len(records) > MaxRoleConnectionMetadata {
		return nil, fmt.Errorf("%w (got %d)", ErrTooManyMetadataRecords, len(records))
	}
	var out []RoleConnectionMetadata
	err := c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/applications/" + appID.String() + "/role-connections/metadata",
		Route:  "PUT /applications/{id}/role-connections/metadata",
		Body:   records,
	}, &out)
	return out, err
}

// UserRoleConnection reads the card currently published for the user that
// bearerToken belongs to.
//
// This endpoint is per-user, not per-bot: it needs an OAuth2 access token with
// the role_connections.write scope, which BearerToken wraps. A bot token works
// too, but then "@me" is the bot's own account and the card lands on the bot's
// profile.
func (c *Client) UserRoleConnection(ctx context.Context, bearerToken string, appID Snowflake) (*RoleConnection, error) {
	var out RoleConnection
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/users/@me/applications/" + appID.String() + "/role-connection",
		Route:  "GET /users/@me/applications/{id}/role-connection",
		Auth:   BearerToken(bearerToken),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateUserRoleConnection publishes the card onto the profile of the user
// that bearerToken belongs to. This is the half that actually makes something
// appear.
//
// The keys in conn.Metadata must match keys registered with
// RegisterRoleConnectionMetadata; unregistered keys are dropped.
func (c *Client) UpdateUserRoleConnection(ctx context.Context, bearerToken string, appID Snowflake, conn RoleConnection) (*RoleConnection, error) {
	var out RoleConnection
	err := c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/users/@me/applications/" + appID.String() + "/role-connection",
		Route:  "PUT /users/@me/applications/{id}/role-connection",
		Auth:   BearerToken(bearerToken),
		Body:   conn,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteUserRoleConnection removes the card from the user's profile.
func (c *Client) DeleteUserRoleConnection(ctx context.Context, bearerToken string, appID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/users/@me/applications/" + appID.String() + "/role-connection",
		Route:  "DELETE /users/@me/applications/{id}/role-connection",
		Auth:   BearerToken(bearerToken),
	}, nil)
}

// BearerToken formats an OAuth2 access token as an Authorization header value.
// It is idempotent, so a token that already carries the prefix is left alone.
func BearerToken(token string) string {
	if len(token) >= 7 && (token[:7] == "Bearer " || token[:7] == "bearer ") {
		return "Bearer " + token[7:]
	}
	return "Bearer " + token
}
