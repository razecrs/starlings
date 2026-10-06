package starlings

import "time"

// Guild is a Discord server.
//
// On the GUILD_CREATE that arrives right after connecting, the gateway fills in
// Channels, Members and the other collections below. Guilds fetched over REST
// carry only the guild's own fields.
type Guild struct {
	ID                          Snowflake    `json:"id"`
	Name                        string       `json:"name"`
	Icon                        string       `json:"icon"`
	Splash                      string       `json:"splash"`
	OwnerID                     Snowflake    `json:"owner_id"`
	Permissions                 Permissions  `json:"permissions,string"`
	AFKChannelID                Snowflake    `json:"afk_channel_id"`
	AFKTimeout                  int          `json:"afk_timeout"`
	VerificationLevel           int          `json:"verification_level"`
	DefaultMessageNotifications int          `json:"default_message_notifications"`
	ExplicitContentFilter       int          `json:"explicit_content_filter"`
	Roles                       []Role       `json:"roles"`
	Emojis                      []GuildEmoji `json:"emojis"`
	Stickers                    []Sticker    `json:"stickers"`
	Features                    []string     `json:"features"`
	MFALevel                    int          `json:"mfa_level"`
	SystemChannelID             Snowflake    `json:"system_channel_id"`
	RulesChannelID              Snowflake    `json:"rules_channel_id"`
	Description                 string       `json:"description"`
	Banner                      string       `json:"banner"`
	PremiumTier                 int          `json:"premium_tier"`
	PremiumSubscriptionCount    int          `json:"premium_subscription_count"`
	PreferredLocale             string       `json:"preferred_locale"`
	NSFWLevel                   int          `json:"nsfw_level"`
	Unavailable                 bool         `json:"unavailable"`

	// Gateway-only, present on GUILD_CREATE.
	JoinedAt             time.Time         `json:"joined_at"`
	Large                bool              `json:"large"`
	MemberCount          int               `json:"member_count"`
	VoiceStates          []VoiceState      `json:"voice_states"`
	Members              []Member          `json:"members"`
	Channels             []Channel         `json:"channels"`
	Threads              []Channel         `json:"threads"`
	Presences            []PresenceUpdate  `json:"presences"`
	StageInstances       []StageInstance   `json:"stage_instances"`
	GuildScheduledEvents []ScheduledEvent  `json:"guild_scheduled_events"`
	SoundboardSounds     []SoundboardSound `json:"soundboard_sounds"`
}

// IconURL returns a CDN link to the guild's icon, or "" if it has none.
func (g *Guild) IconURL(size int) string {
	if g.Icon == "" {
		return ""
	}
	return cdnURL("icons/"+g.ID.String()+"/"+g.Icon, g.Icon, size)
}

// Role returns the role with the given ID, or nil. Only useful on a guild that
// came with its roles populated.
func (g *Guild) Role(id Snowflake) *Role {
	for i := range g.Roles {
		if g.Roles[i].ID == id {
			return &g.Roles[i]
		}
	}
	return nil
}

// VoiceState is a member's current voice connection within a guild.
type VoiceState struct {
	GuildID    Snowflake `json:"guild_id"`
	ChannelID  Snowflake `json:"channel_id"` // zero when the member has disconnected
	UserID     Snowflake `json:"user_id"`
	Member     *Member   `json:"member"`
	SessionID  string    `json:"session_id"`
	Deaf       bool      `json:"deaf"`
	Mute       bool      `json:"mute"`
	SelfDeaf   bool      `json:"self_deaf"`
	SelfMute   bool      `json:"self_mute"`
	SelfStream bool      `json:"self_stream"`
	SelfVideo  bool      `json:"self_video"`
	Suppress   bool      `json:"suppress"`
}
