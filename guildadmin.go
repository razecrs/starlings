package starlings

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/url"
	"time"
)

// AuditLogQuery narrows an audit log request.
type AuditLogQuery struct {
	UserID     Snowflake // only entries caused by this user
	ActionType int       // only this kind of change
	Before     Snowflake
	After      Snowflake
	Limit      int // 1-100
}

// AuditLogEntry is one recorded administrative action.
type AuditLogEntry struct {
	ID         Snowflake        `json:"id"`
	TargetID   string           `json:"target_id"`
	UserID     Snowflake        `json:"user_id"`
	ActionType int              `json:"action_type"`
	Reason     string           `json:"reason"`
	Changes    []AuditLogChange `json:"changes"`
}

// AuditLogChange records one field that an action altered.
type AuditLogChange struct {
	Key      string `json:"key"`
	OldValue any    `json:"old_value"`
	NewValue any    `json:"new_value"`
}

// AuditLog is a page of audit log entries plus the objects they refer to.
type AuditLog struct {
	Entries      []AuditLogEntry `json:"audit_log_entries"`
	Users        []User          `json:"users"`
	Webhooks     []Webhook       `json:"webhooks"`
	Integrations []Integration   `json:"integrations"`
	Threads      []Channel       `json:"threads"`
}

// AuditLog reads a guild's audit log, newest first.
func (c *Client) AuditLog(ctx context.Context, guildID Snowflake, q AuditLogQuery) (*AuditLog, error) {
	v := url.Values{}
	if !q.UserID.IsZero() {
		v.Set("user_id", q.UserID.String())
	}
	if q.ActionType > 0 {
		v.Set("action_type", itoa(q.ActionType))
	}
	if !q.Before.IsZero() {
		v.Set("before", q.Before.String())
	}
	if !q.After.IsZero() {
		v.Set("after", q.After.String())
	}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}

	var out AuditLog
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/audit-logs" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/audit-logs",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Integration is a connection between a guild and an external service.
type Integration struct {
	ID      Snowflake `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Enabled bool      `json:"enabled"`
	User    *User     `json:"user"`
	Account struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"account"`
}

// Integrations lists a guild's integrations.
func (c *Client) Integrations(ctx context.Context, guildID Snowflake) ([]Integration, error) {
	var out []Integration
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/integrations",
		Route:  "GET /guilds/" + guildID.String() + "/integrations",
	}, &out)
	return out, err
}

// DeleteIntegration removes an integration, along with any subscriptions and
// synced roles it created.
func (c *Client) DeleteIntegration(ctx context.Context, guildID, integrationID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/integrations/" + integrationID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/integrations/{id}",
		Reason: reason,
	}, nil)
}

// GuildWidgetSettings controls the embeddable server widget.
type GuildWidgetSettings struct {
	Enabled   bool      `json:"enabled"`
	ChannelID Snowflake `json:"channel_id"`
}

// WidgetSettings reads the server widget settings.
func (c *Client) WidgetSettings(ctx context.Context, guildID Snowflake) (*GuildWidgetSettings, error) {
	var out GuildWidgetSettings
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/widget",
		Route:  "GET /guilds/" + guildID.String() + "/widget",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyWidgetSettings changes the server widget settings.
func (c *Client) ModifyWidgetSettings(ctx context.Context, guildID Snowflake, s GuildWidgetSettings, reason string) (*GuildWidgetSettings, error) {
	var out GuildWidgetSettings
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/widget",
		Route:  "PATCH /guilds/" + guildID.String() + "/widget",
		Body:   s,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// WelcomeScreen is the panel new members see when joining a community guild.
type WelcomeScreen struct {
	Description     string                 `json:"description"`
	WelcomeChannels []WelcomeScreenChannel `json:"welcome_channels"`
}

// WelcomeScreenChannel is one suggested channel on the welcome screen.
type WelcomeScreenChannel struct {
	ChannelID   Snowflake `json:"channel_id"`
	Description string    `json:"description"`
	EmojiID     Snowflake `json:"emoji_id"`
	EmojiName   string    `json:"emoji_name"`
}

// WelcomeScreen reads a guild's welcome screen.
func (c *Client) WelcomeScreen(ctx context.Context, guildID Snowflake) (*WelcomeScreen, error) {
	var out WelcomeScreen
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/welcome-screen",
		Route:  "GET /guilds/" + guildID.String() + "/welcome-screen",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyWelcomeScreen edits a guild's welcome screen.
func (c *Client) ModifyWelcomeScreen(ctx context.Context, guildID Snowflake, s WelcomeScreen, reason string) (*WelcomeScreen, error) {
	var out WelcomeScreen
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/welcome-screen",
		Route:  "PATCH /guilds/" + guildID.String() + "/welcome-screen",
		Body:   s,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// VanityURL is a guild's custom invite code and how many times it was used.
type VanityURL struct {
	Code string `json:"code"`
	Uses int    `json:"uses"`
}

// VanityURL reads a guild's vanity invite. Requires a boosted guild.
func (c *Client) VanityURL(ctx context.Context, guildID Snowflake) (*VanityURL, error) {
	var out VanityURL
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/vanity-url",
		Route:  "GET /guilds/" + guildID.String() + "/vanity-url",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GuildPreview is the public summary of a discoverable guild, readable
// without being a member.
type GuildPreview struct {
	ID                       Snowflake    `json:"id"`
	Name                     string       `json:"name"`
	Icon                     string       `json:"icon"`
	Splash                   string       `json:"splash"`
	DiscoverySplash          string       `json:"discovery_splash"`
	Emojis                   []GuildEmoji `json:"emojis"`
	Features                 []string     `json:"features"`
	ApproximateMemberCount   int          `json:"approximate_member_count"`
	ApproximatePresenceCount int          `json:"approximate_presence_count"`
	Description              string       `json:"description"`
}

// GuildPreview reads a discoverable guild's public preview.
func (c *Client) GuildPreview(ctx context.Context, guildID Snowflake) (*GuildPreview, error) {
	var out GuildPreview
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/preview",
		Route:  "GET /guilds/" + guildID.String() + "/preview",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// VoiceRegion is a server region available for voice channels.
type VoiceRegion struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Optimal    bool   `json:"optimal"`
	Deprecated bool   `json:"deprecated"`
	Custom     bool   `json:"custom"`
}

// VoiceRegions lists the voice regions available generally.
func (c *Client) VoiceRegions(ctx context.Context) ([]VoiceRegion, error) {
	var out []VoiceRegion
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/voice/regions",
		Route:  "GET /voice/regions",
	}, &out)
	return out, err
}

// GuildVoiceRegions lists the voice regions available to one guild, which can
// include VIP regions the guild has unlocked.
func (c *Client) GuildVoiceRegions(ctx context.Context, guildID Snowflake) ([]VoiceRegion, error) {
	var out []VoiceRegion
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/regions",
		Route:  "GET /guilds/" + guildID.String() + "/regions",
	}, &out)
	return out, err
}

// Scheduled events

// ScheduledEventStatus tracks an event through its lifecycle.
const (
	EventScheduled = 1
	EventActive    = 2
	EventCompleted = 3
	EventCancelled = 4
)

// ScheduledEventEntityType says where an event happens.
const (
	EventStageInstance = 1
	EventVoice         = 2
	EventExternal      = 3 // somewhere outside Discord; needs EntityMetadata
)

// ScheduledEventMetadata holds the location of an external scheduled event.
// It is an alias, so code written against the earlier anonymous struct still
// compiles.
type ScheduledEventMetadata = struct {
	Location string `json:"location"`
}

// ScheduledEvent is a planned guild event.
type ScheduledEvent struct {
	ID                 Snowflake               `json:"id"`
	GuildID            Snowflake               `json:"guild_id"`
	ChannelID          Snowflake               `json:"channel_id,omitzero"`
	CreatorID          Snowflake               `json:"creator_id,omitzero"`
	Name               string                  `json:"name"`
	Description        string                  `json:"description,omitzero"`
	ScheduledStartTime time.Time               `json:"scheduled_start_time"`
	ScheduledEndTime   *time.Time              `json:"scheduled_end_time,omitzero"`
	PrivacyLevel       int                     `json:"privacy_level"` // always 2, guild-only
	Status             int                     `json:"status,omitzero"`
	EntityType         int                     `json:"entity_type"`
	EntityID           Snowflake               `json:"entity_id,omitzero"`
	EntityMetadata     *ScheduledEventMetadata `json:"entity_metadata,omitzero"`
	Creator            *User                   `json:"creator,omitzero"`
	UserCount          int                     `json:"user_count,omitzero"`
	Image              string                  `json:"image,omitzero"`
}

// ScheduledEventUpdate is a partial scheduled-event PATCH. Zero values are
// omitted, so changing one field never accidentally sends empty required
// create fields. Clear flags encode JSON null for Discord's nullable fields.
type ScheduledEventUpdate struct {
	ChannelID          *Snowflake              `json:"channel_id,omitzero"`
	Name               string                  `json:"name,omitzero"`
	Description        *string                 `json:"description,omitzero"`
	ScheduledStartTime *time.Time              `json:"scheduled_start_time,omitzero"`
	ScheduledEndTime   *time.Time              `json:"scheduled_end_time,omitzero"`
	PrivacyLevel       int                     `json:"privacy_level,omitzero"`
	Status             int                     `json:"status,omitzero"`
	EntityType         int                     `json:"entity_type,omitzero"`
	EntityMetadata     *ScheduledEventMetadata `json:"entity_metadata,omitzero"`
	Image              *string                 `json:"image,omitzero"`

	ClearChannel        bool `json:"-"`
	ClearScheduledEnd   bool `json:"-"`
	ClearEntityMetadata bool `json:"-"`
}

func (u ScheduledEventUpdate) MarshalJSON() ([]byte, error) {
	type plain ScheduledEventUpdate
	base, err := json.Marshal(plain(u))
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(base, &fields); err != nil {
		return nil, err
	}
	if u.ClearChannel {
		fields["channel_id"] = nil
	}
	if u.ClearScheduledEnd {
		fields["scheduled_end_time"] = nil
	}
	if u.ClearEntityMetadata {
		fields["entity_metadata"] = nil
	}
	return json.Marshal(fields)
}

// ScheduledEvents lists a guild's scheduled events.
func (c *Client) ScheduledEvents(ctx context.Context, guildID Snowflake, withUserCount bool) ([]ScheduledEvent, error) {
	v := url.Values{}
	if withUserCount {
		v.Set("with_user_count", "true")
	}

	var out []ScheduledEvent
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/scheduled-events" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/scheduled-events",
	}, &out)
	return out, err
}

// ScheduledEvent fetches one scheduled event.
func (c *Client) ScheduledEvent(ctx context.Context, guildID, eventID Snowflake, withUserCount bool) (*ScheduledEvent, error) {
	v := url.Values{}
	if withUserCount {
		v.Set("with_user_count", "true")
	}

	var out ScheduledEvent
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/scheduled-events/" + eventID.String() + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/scheduled-events/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateScheduledEvent schedules a guild event.
//
// An EventExternal event must supply both EntityMetadata.Location and
// ScheduledEndTime; the other kinds need ChannelID instead.
func (c *Client) CreateScheduledEvent(ctx context.Context, guildID Snowflake, e ScheduledEvent, reason string) (*ScheduledEvent, error) {
	if e.PrivacyLevel == 0 {
		e.PrivacyLevel = 2 // the only value Discord accepts
	}

	var out ScheduledEvent
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/scheduled-events",
		Route:  "POST /guilds/" + guildID.String() + "/scheduled-events",
		Body:   e,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyScheduledEvent edits an event. Setting Status to EventActive starts
// it and EventCompleted ends it.
func (c *Client) ModifyScheduledEvent(ctx context.Context, guildID, eventID Snowflake, e ScheduledEvent, reason string) (*ScheduledEvent, error) {
	var out ScheduledEvent
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/scheduled-events/" + eventID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/scheduled-events/{id}",
		Body:   e,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateScheduledEvent edits only the supplied fields. Prefer this over
// ModifyScheduledEvent, whose whole-object payload is retained for v0.1.0
// compatibility.
func (c *Client) UpdateScheduledEvent(ctx context.Context, guildID, eventID Snowflake, update ScheduledEventUpdate, reason string) (*ScheduledEvent, error) {
	var out ScheduledEvent
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/scheduled-events/" + eventID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/scheduled-events/{id}",
		Body:   update,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteScheduledEvent cancels and removes an event.
func (c *Client) DeleteScheduledEvent(ctx context.Context, guildID, eventID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/scheduled-events/" + eventID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/scheduled-events/{id}",
	}, nil)
}

// ScheduledEventUser is someone subscribed to an event.
type ScheduledEventUser struct {
	GuildScheduledEventID Snowflake `json:"guild_scheduled_event_id"`
	User                  *User     `json:"user"`
	Member                *Member   `json:"member"`
}

// ScheduledEventUsers lists who is interested in an event.
func (c *Client) ScheduledEventUsers(ctx context.Context, guildID, eventID Snowflake, limit int, withMember bool) ([]ScheduledEventUser, error) {
	v := url.Values{}
	if limit > 0 {
		v.Set("limit", itoa(limit))
	}
	if withMember {
		v.Set("with_member", "true")
	}

	var out []ScheduledEventUser
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/scheduled-events/" + eventID.String() + "/users" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/scheduled-events/{id}/users",
	}, &out)
	return out, err
}
