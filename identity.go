package starlings

import (
	"context"
	"net/http"
)

// Application Identity Profiles are the data behind Game Stats Widgets - the
// cards a game can put on a player's Discord profile showing rank, playtime,
// wins and anything else it defines.
//
// Discord documents these at
// https://docs.discord.com/developers/resources/application-identity-profile
// and https://docs.discord.com/developers/social-layer/game-stats-widgets.
//
// Three things gate the API, and all three are required:
//
//   - the application has the Social SDK enabled and its terms agreed;
//   - the game is claimed on Discord, which needs a public Steam store page
//     carrying a Discord invite plus enough detected playtime for Discord to
//     hold a record of the game - bots and unreleased projects cannot claim;
//   - the player has authorised the application with the
//     ScopeApplicationIdentitiesWrite scope.
//
// The calls themselves authenticate with an ordinary bot token, which is why
// a plain server-side program can drive them.

// ScopeApplicationIdentitiesWrite is the OAuth2 scope a player must grant
// before an application may read or write their identity profile. The Social
// SDK's own scope set already includes it.
const ScopeApplicationIdentitiesWrite = "application_identities.write"

// Media is an image referenced by URL.
//
// Discord's unfurler fetches the URL from its own servers, not from the
// viewer's client, so it has to be reachable from the public internet -
// localhost and LAN addresses never load.
type Media struct {
	URL string `json:"url"`
}

// DynamicFieldType identifies the value shape of a custom stat.
type DynamicFieldType int

const (
	DynamicString DynamicFieldType = iota + 1
	DynamicNumber
	DynamicMedia
)

// DynamicField is a custom stat, for anything the primary fields do not cover.
//
// Name is the key referenced by a User Data field in the widget editor. It is
// never shown to players - display labels live on the widget's fields, not
// here.
type DynamicField struct {
	Type  DynamicFieldType `json:"type"`
	Name  string           `json:"name"`
	Value any              `json:"value"` // string, number, or Media
}

// StringField builds a text stat.
func StringField(name, value string) DynamicField {
	return DynamicField{Type: DynamicString, Name: name, Value: value}
}

// NumberField builds a numeric stat. The widget editor can render one with
// compact notation, or as a duration when the value is milliseconds.
func NumberField(name string, value float64) DynamicField {
	return DynamicField{Type: DynamicNumber, Name: name, Value: value}
}

// MediaField builds an image stat - the mechanism behind widget tiles that
// show a thumbnail alongside their text.
func MediaField(name, url string) DynamicField {
	return DynamicField{Type: DynamicMedia, Name: name, Value: Media{URL: url}}
}

// PrimaryProfileData is Discord's set of pre-defined stats, meant to be
// generic across games. Every field is optional; populate the ones that suit
// the game and leave the rest zero.
type PrimaryProfileData struct {
	Season                       string  `json:"season,omitzero"`
	RankName                     string  `json:"rank_name,omitzero"`
	RankImage                    *Media  `json:"rank_image,omitzero"`
	HighestRank                  string  `json:"highest_rank,omitzero"`
	HighestRankImage             *Media  `json:"highest_rank_image,omitzero"`
	FeaturedPlayedCharacter      string  `json:"featured_played_character,omitzero"`
	FeaturedPlayedCharacterImage *Media  `json:"featured_played_character_image,omitzero"`
	PlaytimeHours                float64 `json:"playtime_hours,omitzero"` // decimals allowed
	TotalWins                    int     `json:"total_wins,omitzero"`
	CurrentPeriodWins            int     `json:"current_period_wins,omitzero"`
	TotalGames                   int     `json:"total_games,omitzero"`
	CurrentPeriodGames           int     `json:"current_period_games,omitzero"`
	TotalKills                   int     `json:"total_kills,omitzero"`
	CurrentPeriodKills           int     `json:"current_period_kills,omitzero"`
	TotalAssists                 int     `json:"total_assists,omitzero"`
	CurrentPeriodAssists         int     `json:"current_period_assists,omitzero"`
	TotalDeaths                  int     `json:"total_deaths,omitzero"`
	CurrentPeriodDeaths          int     `json:"current_period_deaths,omitzero"`
}

// ProfileData holds the stats themselves.
type ProfileData struct {
	Primary *PrimaryProfileData `json:"primary,omitzero"`
	Dynamic []DynamicField      `json:"dynamic,omitzero"`
}

// IdentityProfile is a player's record for one application.
//
// Metadata is game-defined and never read by Discord; it is storage the
// application gets for free alongside the stats.
type IdentityProfile struct {
	Username string         `json:"username,omitzero"`
	Metadata map[string]any `json:"metadata,omitzero"`
	Data     *ProfileData   `json:"data,omitzero"`
}

// ApplicationIdentity is a user's identity in an application, keyed by which
// external system vouched for them.
type ApplicationIdentity struct {
	ProviderType         string `json:"provider_type"`
	ProviderID           string `json:"provider_id,omitzero"`
	ProviderIssuedUserID string `json:"provider_issued_user_id"`
}

// SetIdentityProfile writes a player's stats.
//
// providerIssuedUserID is the player's ID in the game's own system. When the
// user has no identity for this application yet, the first successful write
// creates one with provider type NONE using this value; when they already have
// one, it must match an identity they already have.
//
// Data is replaced wholesale: any field left out of profile.Data is erased, so
// always send the complete set. Leaving profile.Data nil instead updates the
// surrounding fields and leaves the stored stats untouched.
func (c *Client) SetIdentityProfile(ctx context.Context, appID, userID Snowflake, providerIssuedUserID string, profile IdentityProfile) (*IdentityProfile, error) {
	var out IdentityProfile
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path: "/applications/" + appID.String() + "/users/" + userID.String() +
			"/identities/" + providerIssuedUserID + "/profile",
		Route: "PATCH /applications/{id}/users/{id}/identities/{id}/profile",
		Body:  profile,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// IdentityProfile reads back a player's stored stats.
func (c *Client) IdentityProfile(ctx context.Context, appID, userID Snowflake, providerIssuedUserID string) (*IdentityProfile, error) {
	var out IdentityProfile
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/applications/" + appID.String() + "/users/" + userID.String() +
			"/identities/" + providerIssuedUserID + "/profile",
		Route: "GET /applications/{id}/users/{id}/identities/{id}/profile",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UserApplicationIdentities lists the identities a user holds for one
// application. A bot may only ask about its own application.
func (c *Client) UserApplicationIdentities(ctx context.Context, userID, appID Snowflake) ([]ApplicationIdentity, error) {
	var resp struct {
		Identities []ApplicationIdentity `json:"identities"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/users/" + userID.String() + "/application-identities/" + appID.String(),
		Route:  "GET /users/{id}/application-identities/{id}",
	}, &resp)
	return resp.Identities, err
}

// LookupApplicationIdentity finds the identity matching an external account,
// which is how a game maps its own user ID back to a Discord user.
func (c *Client) LookupApplicationIdentity(ctx context.Context, appID Snowflake, providerType, providerIssuedUserID string) (*ApplicationIdentity, error) {
	var resp struct {
		Identities []ApplicationIdentity `json:"identities"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/applications/" + appID.String() + "/application-identities/" +
			providerType + "/" + providerIssuedUserID,
		Route: "GET /applications/{id}/application-identities/{type}/{id}",
	}, &resp)
	if err != nil {
		return nil, err
	}
	if len(resp.Identities) == 0 {
		return nil, nil
	}
	return &resp.Identities[0], nil
}

// DeleteApplicationIdentity removes a user's identity, and the profile stored
// against it, for this application.
func (c *Client) DeleteApplicationIdentity(ctx context.Context, userID, appID Snowflake, providerType, providerIssuedUserID string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path: "/users/" + userID.String() + "/application-identities/" + appID.String() +
			"/" + providerType + "/" + providerIssuedUserID + "/delete",
		Route: "POST /users/{id}/application-identities/{id}/{type}/{id}/delete",
	}, nil)
}

// CurrentApplication describes the application a bot token belongs to.
type CurrentApplication struct {
	ID          Snowflake `json:"id"`
	Name        string    `json:"name"`
	Icon        string    `json:"icon"`
	Description string    `json:"description"`
	Owner       *User     `json:"owner"`
	Team        *AppTeam  `json:"team"`
}

// AppTeam is the team that owns an application. When an app is team-owned the
// "owner" user Discord returns is a synthetic team account, not a person, so
// OwnerUserID is the only field that names a human.
type AppTeam struct {
	ID          Snowflake `json:"id"`
	Name        string    `json:"name"`
	OwnerUserID Snowflake `json:"owner_user_id"`
}

// HumanOwnerID returns the user ID of the person who owns the application,
// resolving through the team when there is one, and zero if neither is set.
func (a *CurrentApplication) HumanOwnerID() Snowflake {
	if a.Team != nil && !a.Team.OwnerUserID.IsZero() {
		return a.Team.OwnerUserID
	}
	if a.Owner != nil {
		return a.Owner.ID
	}
	return 0
}

// CurrentApplication returns the application behind the bot token, which is
// how a program learns its own application ID without being told.
func (c *Client) CurrentApplication(ctx context.Context) (*CurrentApplication, error) {
	var app CurrentApplication
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/@me",
		Route:  "GET /applications/@me",
	}, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// AuthorizeURL builds the OAuth2 URL a player visits to grant an application
// the given scopes. For game stats widgets that is
// ScopeApplicationIdentitiesWrite, and the redirect URI must already be
// registered on the application.
func AuthorizeURL(appID Snowflake, redirectURI string, scopes ...string) string {
	return "https://discord.com/oauth2/authorize" +
		"?client_id=" + appID.String() +
		"&response_type=code" +
		"&redirect_uri=" + urlEscape(redirectURI) +
		"&scope=" + urlEscape(joinSpace(scopes))
}
