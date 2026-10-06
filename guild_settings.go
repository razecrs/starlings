package starlings

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// GuildTemplate is a snapshot of a guild's channels, roles and settings that
// can be used to create new guilds.
type GuildTemplate struct {
	Code                  string    `json:"code"`
	Name                  string    `json:"name"`
	Description           string    `json:"description"`
	UsageCount            int       `json:"usage_count"`
	CreatorID             Snowflake `json:"creator_id"`
	Creator               *User     `json:"creator"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	SourceGuildID         Snowflake `json:"source_guild_id"`
	SerializedSourceGuild *Guild    `json:"serialized_source_guild"`
	IsDirty               bool      `json:"is_dirty"` // source guild changed since the snapshot
}

// GuildTemplate looks up a template by its share code.
func (c *Client) GuildTemplate(ctx context.Context, code string) (*GuildTemplate, error) {
	var out GuildTemplate
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/templates/" + url.PathEscape(code),
		Route:  "GET /guilds/templates/{code}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GuildTemplates lists a guild's templates. A guild may have only one.
func (c *Client) GuildTemplates(ctx context.Context, guildID Snowflake) ([]GuildTemplate, error) {
	var out []GuildTemplate
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/templates",
		Route:  "GET /guilds/" + guildID.String() + "/templates",
	}, &out)
	return out, err
}

// CreateGuildTemplate snapshots a guild into a new template.
func (c *Client) CreateGuildTemplate(ctx context.Context, guildID Snowflake, name, description string) (*GuildTemplate, error) {
	var out GuildTemplate
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/templates",
		Route:  "POST /guilds/" + guildID.String() + "/templates",
		Body:   map[string]any{"name": name, "description": description},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SyncGuildTemplate refreshes a template from the guild's current state,
// clearing IsDirty.
func (c *Client) SyncGuildTemplate(ctx context.Context, guildID Snowflake, code string) (*GuildTemplate, error) {
	var out GuildTemplate
	err := c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/guilds/" + guildID.String() + "/templates/" + url.PathEscape(code),
		Route:  "PUT /guilds/" + guildID.String() + "/templates/{code}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyGuildTemplate renames a template.
func (c *Client) ModifyGuildTemplate(ctx context.Context, guildID Snowflake, code, name, description string) (*GuildTemplate, error) {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if description != "" {
		body["description"] = description
	}

	var out GuildTemplate
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/templates/" + url.PathEscape(code),
		Route:  "PATCH /guilds/" + guildID.String() + "/templates/{code}",
		Body:   body,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteGuildTemplate removes a template.
func (c *Client) DeleteGuildTemplate(ctx context.Context, guildID Snowflake, code string) (*GuildTemplate, error) {
	var out GuildTemplate
	err := c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/templates/" + url.PathEscape(code),
		Route:  "DELETE /guilds/" + guildID.String() + "/templates/{code}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Voice states

// CurrentVoiceState reads the bot's own voice state in a guild.
func (c *Client) CurrentVoiceState(ctx context.Context, guildID Snowflake) (*VoiceState, error) {
	var out VoiceState
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/voice-states/@me",
		Route:  "GET /guilds/" + guildID.String() + "/voice-states/@me",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// VoiceStateOf reads a member's voice state in a guild.
func (c *Client) VoiceStateOf(ctx context.Context, guildID, userID Snowflake) (*VoiceState, error) {
	var out VoiceState
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/voice-states/" + userID.String(),
		Route:  "GET /guilds/" + guildID.String() + "/voice-states/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetCurrentVoiceState updates the bot's own voice state, which is how a bot
// un-suppresses itself on a stage or asks to speak.
func (c *Client) SetCurrentVoiceState(ctx context.Context, guildID, channelID Snowflake, suppress bool, requestToSpeak *time.Time) error {
	body := map[string]any{"channel_id": channelID, "suppress": suppress}
	if requestToSpeak != nil {
		body["request_to_speak_timestamp"] = requestToSpeak
	}
	return c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/voice-states/@me",
		Route:  "PATCH /guilds/" + guildID.String() + "/voice-states/@me",
		Body:   body,
	}, nil)
}

// SetVoiceState updates another member's voice state - used to invite someone
// to speak on a stage, or move them back to the audience.
func (c *Client) SetVoiceState(ctx context.Context, guildID, userID, channelID Snowflake, suppress bool) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/voice-states/" + userID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/voice-states/{id}",
		Body:   map[string]any{"channel_id": channelID, "suppress": suppress},
	}, nil)
}

// Guild settings

// GuildParams are the settable fields of a guild.
type GuildParams struct {
	Name                        *string    `json:"name,omitzero"`
	VerificationLevel           *int       `json:"verification_level,omitzero"`
	DefaultMessageNotifications *int       `json:"default_message_notifications,omitzero"`
	ExplicitContentFilter       *int       `json:"explicit_content_filter,omitzero"`
	AFKChannelID                *Snowflake `json:"afk_channel_id,omitzero"`
	AFKTimeout                  *int       `json:"afk_timeout,omitzero"`
	Icon                        *string    `json:"icon,omitzero"` // data URI
	OwnerID                     *Snowflake `json:"owner_id,omitzero"`
	Splash                      *string    `json:"splash,omitzero"`
	Banner                      *string    `json:"banner,omitzero"`
	SystemChannelID             *Snowflake `json:"system_channel_id,omitzero"`
	RulesChannelID              *Snowflake `json:"rules_channel_id,omitzero"`
	PublicUpdatesChannelID      *Snowflake `json:"public_updates_channel_id,omitzero"`
	PreferredLocale             *string    `json:"preferred_locale,omitzero"`
	Features                    *[]string  `json:"features,omitzero"`
	Description                 *string    `json:"description,omitzero"`
	PremiumProgressBarEnabled   *bool      `json:"premium_progress_bar_enabled,omitzero"`
}

// ModifyGuild changes a guild's settings.
func (c *Client) ModifyGuild(ctx context.Context, guildID Snowflake, params GuildParams, reason string) (*Guild, error) {
	var out Guild
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String(),
		Route:  "PATCH /guilds/" + guildID.String(),
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GuildOnboarding is the new-member onboarding flow.
type GuildOnboarding struct {
	GuildID           Snowflake          `json:"guild_id"`
	Prompts           []OnboardingPrompt `json:"prompts"`
	DefaultChannelIDs []Snowflake        `json:"default_channel_ids"`
	Enabled           bool               `json:"enabled"`
	Mode              int                `json:"mode"`
}

// OnboardingPrompt is one question asked during onboarding.
type OnboardingPrompt struct {
	ID           Snowflake          `json:"id"`
	Type         int                `json:"type"`
	Options      []OnboardingOption `json:"options"`
	Title        string             `json:"title"`
	SingleSelect bool               `json:"single_select"`
	Required     bool               `json:"required"`
	InOnboarding bool               `json:"in_onboarding"`
}

// OnboardingOption is one answer to an onboarding prompt.
type OnboardingOption struct {
	ID          Snowflake   `json:"id"`
	ChannelIDs  []Snowflake `json:"channel_ids"`
	RoleIDs     []Snowflake `json:"role_ids"`
	Emoji       *Emoji      `json:"emoji,omitzero"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
}

// Onboarding reads a guild's onboarding configuration.
func (c *Client) Onboarding(ctx context.Context, guildID Snowflake) (*GuildOnboarding, error) {
	var out GuildOnboarding
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/onboarding",
		Route:  "GET /guilds/" + guildID.String() + "/onboarding",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetOnboarding replaces a guild's onboarding configuration.
func (c *Client) SetOnboarding(ctx context.Context, guildID Snowflake, o GuildOnboarding, reason string) (*GuildOnboarding, error) {
	var out GuildOnboarding
	err := c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/guilds/" + guildID.String() + "/onboarding",
		Route:  "PUT /guilds/" + guildID.String() + "/onboarding",
		Body:   o,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// NewMemberWelcome is the "start here" panel shown to people who just joined.
type NewMemberWelcome struct {
	GuildID          Snowflake `json:"guild_id"`
	Description      string    `json:"description"`
	NewMemberActions []struct {
		ChannelID   Snowflake `json:"channel_id"`
		ActionType  int       `json:"action_type"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		EmojiID     Snowflake `json:"emoji_id,omitzero"`
		EmojiName   string    `json:"emoji_name,omitzero"`
	} `json:"new_member_actions"`
}

// NewMemberWelcome reads a guild's new-member welcome panel.
func (c *Client) NewMemberWelcome(ctx context.Context, guildID Snowflake) (*NewMemberWelcome, error) {
	var out NewMemberWelcome
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/new-member-welcome",
		Route:  "GET /guilds/" + guildID.String() + "/new-member-welcome",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetIncidentActions pauses invites or DMs during a raid. Both timestamps may
// be at most 24 hours ahead; pass nil to clear a pause.
func (c *Client) SetIncidentActions(ctx context.Context, guildID Snowflake, invitesDisabledUntil, dmsDisabledUntil *time.Time) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/guilds/" + guildID.String() + "/incident-actions",
		Route:  "PUT /guilds/" + guildID.String() + "/incident-actions",
		Body: map[string]any{
			"invites_disabled_until": invitesDisabledUntil,
			"dms_disabled_until":     dmsDisabledUntil,
		},
	}, nil)
}
