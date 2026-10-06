package starlings

import "time"

// The remaining gateway events. They are split from events.go only to keep
// each file a readable length; there is no behavioural difference.

// Threads

// ThreadCreate fires when a thread is created, and when the bot is added to an
// existing one. NewlyCreated distinguishes the two.
type ThreadCreate struct {
	eventBase `json:"-"`
	Channel
	NewlyCreated bool `json:"newly_created"`
}

func (ThreadCreate) eventName() string { return "THREAD_CREATE" }

// ThreadUpdate fires when a thread's settings change. It does not fire for
// the last-message timestamp moving.
type ThreadUpdate struct {
	eventBase `json:"-"`
	Channel
	BeforeUpdate *Channel `json:"-"`
}

func (ThreadUpdate) eventName() string { return "THREAD_UPDATE" }

// ThreadDelete fires when a thread is deleted.
type ThreadDelete struct {
	eventBase    `json:"-"`
	ID           Snowflake   `json:"id"`
	GuildID      Snowflake   `json:"guild_id"`
	ParentID     Snowflake   `json:"parent_id"`
	Type         ChannelType `json:"type"`
	BeforeDelete *Channel    `json:"-"`
}

func (ThreadDelete) eventName() string { return "THREAD_DELETE" }

// ThreadListSync fires when the bot gains access to a channel, listing the
// active threads it can now see.
type ThreadListSync struct {
	eventBase  `json:"-"`
	GuildID    Snowflake      `json:"guild_id"`
	ChannelIDs []Snowflake    `json:"channel_ids"`
	Threads    []Channel      `json:"threads"`
	Members    []ThreadMember `json:"members"`
}

func (ThreadListSync) eventName() string { return "THREAD_LIST_SYNC" }

// ThreadMemberUpdate fires when the bot's own thread membership changes.
type ThreadMemberUpdate struct {
	eventBase `json:"-"`
	ThreadMember
	GuildID      Snowflake     `json:"guild_id"`
	BeforeUpdate *ThreadMember `json:"-"`
}

func (ThreadMemberUpdate) eventName() string { return "THREAD_MEMBER_UPDATE" }

// ThreadMembersUpdate fires when people join or leave a thread. The added and
// removed lists are only populated with the GuildMembers intent.
type ThreadMembersUpdate struct {
	eventBase        `json:"-"`
	ID               Snowflake      `json:"id"`
	GuildID          Snowflake      `json:"guild_id"`
	MemberCount      int            `json:"member_count"`
	AddedMembers     []ThreadMember `json:"added_members"`
	RemovedMemberIDs []Snowflake    `json:"removed_member_ids"`
	BeforeUpdate     []ThreadMember `json:"-"`
}

func (ThreadMembersUpdate) eventName() string { return "THREAD_MEMBERS_UPDATE" }

// ChannelPinsUpdate fires when a message is pinned or unpinned. It carries no
// message, only the fact that the pins changed.
type ChannelPinsUpdate struct {
	eventBase        `json:"-"`
	GuildID          Snowflake  `json:"guild_id"`
	ChannelID        Snowflake  `json:"channel_id"`
	LastPinTimestamp *time.Time `json:"last_pin_timestamp"`
	BeforeUpdate     *Channel   `json:"-"`
}

func (ChannelPinsUpdate) eventName() string { return "CHANNEL_PINS_UPDATE" }

// Guild

// GuildBanAdd fires when a user is banned. Needs the GuildModeration intent.
type GuildBanAdd struct {
	eventBase `json:"-"`
	GuildID   Snowflake `json:"guild_id"`
	User      *User     `json:"user"`
}

func (GuildBanAdd) eventName() string { return "GUILD_BAN_ADD" }

// GuildBanRemove fires when a user is unbanned.
type GuildBanRemove struct {
	eventBase `json:"-"`
	GuildID   Snowflake `json:"guild_id"`
	User      *User     `json:"user"`
}

func (GuildBanRemove) eventName() string { return "GUILD_BAN_REMOVE" }

// GuildAuditLogEntryCreate fires on any logged administrative action, which
// makes it the cheapest way to watch moderation without polling the audit log.
type GuildAuditLogEntryCreate struct {
	eventBase `json:"-"`
	AuditLogEntry
	GuildID Snowflake `json:"guild_id"`
}

func (GuildAuditLogEntryCreate) eventName() string { return "GUILD_AUDIT_LOG_ENTRY_CREATE" }

// GuildEmojisUpdate fires when a guild's emoji change. It sends the whole new
// set rather than a delta.
type GuildEmojisUpdate struct {
	eventBase    `json:"-"`
	GuildID      Snowflake    `json:"guild_id"`
	Emojis       []GuildEmoji `json:"emojis"`
	BeforeUpdate []GuildEmoji `json:"-"`
}

func (GuildEmojisUpdate) eventName() string { return "GUILD_EMOJIS_UPDATE" }

// GuildStickersUpdate fires when a guild's stickers change.
type GuildStickersUpdate struct {
	eventBase    `json:"-"`
	GuildID      Snowflake `json:"guild_id"`
	Stickers     []Sticker `json:"stickers"`
	BeforeUpdate []Sticker `json:"-"`
}

func (GuildStickersUpdate) eventName() string { return "GUILD_STICKERS_UPDATE" }

// GuildIntegrationsUpdate fires when a guild's integrations change.
type GuildIntegrationsUpdate struct {
	eventBase `json:"-"`
	GuildID   Snowflake `json:"guild_id"`
}

func (GuildIntegrationsUpdate) eventName() string { return "GUILD_INTEGRATIONS_UPDATE" }

// GuildMembersChunk answers a Request Guild Members call. Large member lists
// arrive across several chunks; ChunkIndex and ChunkCount say where this one
// sits.
type GuildMembersChunk struct {
	eventBase  `json:"-"`
	GuildID    Snowflake        `json:"guild_id"`
	Members    []Member         `json:"members"`
	ChunkIndex int              `json:"chunk_index"`
	ChunkCount int              `json:"chunk_count"`
	NotFound   []Snowflake      `json:"not_found"`
	Presences  []PresenceUpdate `json:"presences"`
	Nonce      string           `json:"nonce"`
}

func (GuildMembersChunk) eventName() string { return "GUILD_MEMBERS_CHUNK" }

// GuildRoleCreate fires when a role is added.
type GuildRoleCreate struct {
	eventBase `json:"-"`
	GuildID   Snowflake `json:"guild_id"`
	Role      *Role     `json:"role"`
}

func (GuildRoleCreate) eventName() string { return "GUILD_ROLE_CREATE" }

// GuildRoleUpdate fires when a role changes.
type GuildRoleUpdate struct {
	eventBase    `json:"-"`
	GuildID      Snowflake `json:"guild_id"`
	Role         *Role     `json:"role"`
	BeforeUpdate *Role     `json:"-"`
}

func (GuildRoleUpdate) eventName() string { return "GUILD_ROLE_UPDATE" }

// GuildRoleDelete fires when a role is removed.
type GuildRoleDelete struct {
	eventBase    `json:"-"`
	GuildID      Snowflake `json:"guild_id"`
	RoleID       Snowflake `json:"role_id"`
	BeforeDelete *Role     `json:"-"`
}

func (GuildRoleDelete) eventName() string { return "GUILD_ROLE_DELETE" }

// Scheduled events

// GuildScheduledEventCreate fires when an event is scheduled.
type GuildScheduledEventCreate struct {
	eventBase `json:"-"`
	ScheduledEvent
}

func (GuildScheduledEventCreate) eventName() string { return "GUILD_SCHEDULED_EVENT_CREATE" }

// GuildScheduledEventUpdate fires when an event changes, including when it
// starts and ends - watch Status for that.
type GuildScheduledEventUpdate struct {
	eventBase `json:"-"`
	ScheduledEvent
}

func (GuildScheduledEventUpdate) eventName() string { return "GUILD_SCHEDULED_EVENT_UPDATE" }

// GuildScheduledEventDelete fires when an event is cancelled.
type GuildScheduledEventDelete struct {
	eventBase `json:"-"`
	ScheduledEvent
}

func (GuildScheduledEventDelete) eventName() string { return "GUILD_SCHEDULED_EVENT_DELETE" }

// GuildScheduledEventUserAdd fires when someone marks themselves interested.
type GuildScheduledEventUserAdd struct {
	eventBase             `json:"-"`
	GuildScheduledEventID Snowflake `json:"guild_scheduled_event_id"`
	UserID                Snowflake `json:"user_id"`
	GuildID               Snowflake `json:"guild_id"`
}

func (GuildScheduledEventUserAdd) eventName() string { return "GUILD_SCHEDULED_EVENT_USER_ADD" }

// GuildScheduledEventUserRemove fires when someone withdraws interest.
type GuildScheduledEventUserRemove struct {
	eventBase             `json:"-"`
	GuildScheduledEventID Snowflake `json:"guild_scheduled_event_id"`
	UserID                Snowflake `json:"user_id"`
	GuildID               Snowflake `json:"guild_id"`
}

func (GuildScheduledEventUserRemove) eventName() string {
	return "GUILD_SCHEDULED_EVENT_USER_REMOVE"
}

// Soundboard

// GuildSoundboardSoundCreate fires when a sound is uploaded.
type GuildSoundboardSoundCreate struct {
	eventBase `json:"-"`
	SoundboardSound
}

func (GuildSoundboardSoundCreate) eventName() string { return "GUILD_SOUNDBOARD_SOUND_CREATE" }

// GuildSoundboardSoundUpdate fires when a sound is edited.
type GuildSoundboardSoundUpdate struct {
	eventBase `json:"-"`
	SoundboardSound
}

func (GuildSoundboardSoundUpdate) eventName() string { return "GUILD_SOUNDBOARD_SOUND_UPDATE" }

// GuildSoundboardSoundDelete fires when a sound is removed.
type GuildSoundboardSoundDelete struct {
	eventBase `json:"-"`
	SoundID   Snowflake `json:"sound_id"`
	GuildID   Snowflake `json:"guild_id"`
}

func (GuildSoundboardSoundDelete) eventName() string { return "GUILD_SOUNDBOARD_SOUND_DELETE" }

// GuildSoundboardSoundsUpdate fires with the guild's whole sound list after a
// bulk change.
type GuildSoundboardSoundsUpdate struct {
	eventBase        `json:"-"`
	GuildID          Snowflake         `json:"guild_id"`
	SoundboardSounds []SoundboardSound `json:"soundboard_sounds"`
}

func (GuildSoundboardSoundsUpdate) eventName() string { return "GUILD_SOUNDBOARD_SOUNDS_UPDATE" }

// SoundboardSounds answers a Request Soundboard Sounds call.
type SoundboardSounds struct {
	eventBase        `json:"-"`
	GuildID          Snowflake         `json:"guild_id"`
	SoundboardSounds []SoundboardSound `json:"soundboard_sounds"`
}

func (SoundboardSounds) eventName() string { return "SOUNDBOARD_SOUNDS" }

// Integrations, invites and webhooks

// IntegrationCreate fires when an integration is added to a guild.
type IntegrationCreate struct {
	eventBase `json:"-"`
	Integration
	GuildID Snowflake `json:"guild_id"`
}

func (IntegrationCreate) eventName() string { return "INTEGRATION_CREATE" }

// IntegrationUpdate fires when an integration changes.
type IntegrationUpdate struct {
	eventBase `json:"-"`
	Integration
	GuildID Snowflake `json:"guild_id"`
}

func (IntegrationUpdate) eventName() string { return "INTEGRATION_UPDATE" }

// IntegrationDelete fires when an integration is removed.
type IntegrationDelete struct {
	eventBase     `json:"-"`
	ID            Snowflake `json:"id"`
	GuildID       Snowflake `json:"guild_id"`
	ApplicationID Snowflake `json:"application_id"`
}

func (IntegrationDelete) eventName() string { return "INTEGRATION_DELETE" }

// InviteCreate fires when an invite is made. Needs the GuildInvites intent.
type InviteCreate struct {
	eventBase  `json:"-"`
	ChannelID  Snowflake `json:"channel_id"`
	Code       string    `json:"code"`
	CreatedAt  time.Time `json:"created_at"`
	GuildID    Snowflake `json:"guild_id"`
	Inviter    *User     `json:"inviter"`
	MaxAge     int       `json:"max_age"`
	MaxUses    int       `json:"max_uses"`
	Temporary  bool      `json:"temporary"`
	Uses       int       `json:"uses"`
	TargetType int       `json:"target_type"`
	TargetUser *User     `json:"target_user"`
}

func (InviteCreate) eventName() string { return "INVITE_CREATE" }

// InviteDelete fires when an invite is revoked or expires.
type InviteDelete struct {
	eventBase `json:"-"`
	ChannelID Snowflake `json:"channel_id"`
	GuildID   Snowflake `json:"guild_id"`
	Code      string    `json:"code"`
}

func (InviteDelete) eventName() string { return "INVITE_DELETE" }

// WebhooksUpdate fires when a channel's webhooks change. Like
// ChannelPinsUpdate it carries no detail, only the fact of the change.
type WebhooksUpdate struct {
	eventBase `json:"-"`
	GuildID   Snowflake `json:"guild_id"`
	ChannelID Snowflake `json:"channel_id"`
}

func (WebhooksUpdate) eventName() string { return "WEBHOOKS_UPDATE" }

// Auto moderation

// AutoModerationRuleCreate fires when a rule is added.
type AutoModerationRuleCreate struct {
	eventBase `json:"-"`
	AutomodRule
}

func (AutoModerationRuleCreate) eventName() string { return "AUTO_MODERATION_RULE_CREATE" }

// AutoModerationRuleUpdate fires when a rule changes.
type AutoModerationRuleUpdate struct {
	eventBase `json:"-"`
	AutomodRule
}

func (AutoModerationRuleUpdate) eventName() string { return "AUTO_MODERATION_RULE_UPDATE" }

// AutoModerationRuleDelete fires when a rule is removed.
type AutoModerationRuleDelete struct {
	eventBase `json:"-"`
	AutomodRule
}

func (AutoModerationRuleDelete) eventName() string { return "AUTO_MODERATION_RULE_DELETE" }

// AutoModerationActionExecution fires when a rule matches and acts. Needs the
// AutoModerationExecution intent, which is separate from the configuration
// one.
type AutoModerationActionExecution struct {
	eventBase            `json:"-"`
	GuildID              Snowflake     `json:"guild_id"`
	Action               AutomodAction `json:"action"`
	RuleID               Snowflake     `json:"rule_id"`
	RuleTriggerType      int           `json:"rule_trigger_type"`
	UserID               Snowflake     `json:"user_id"`
	ChannelID            Snowflake     `json:"channel_id"`
	MessageID            Snowflake     `json:"message_id"`
	AlertSystemMessageID Snowflake     `json:"alert_system_message_id"`
	Content              string        `json:"content"`
	MatchedKeyword       string        `json:"matched_keyword"`
	MatchedContent       string        `json:"matched_content"`
}

func (AutoModerationActionExecution) eventName() string { return "AUTO_MODERATION_ACTION_EXECUTION" }

// Stage and presence

// StageInstanceCreate fires when a stage channel goes live.
type StageInstanceCreate struct {
	eventBase `json:"-"`
	StageInstance
}

func (StageInstanceCreate) eventName() string { return "STAGE_INSTANCE_CREATE" }

// StageInstanceUpdate fires when a live stage changes.
type StageInstanceUpdate struct {
	eventBase `json:"-"`
	StageInstance
}

func (StageInstanceUpdate) eventName() string { return "STAGE_INSTANCE_UPDATE" }

// StageInstanceDelete fires when a stage ends.
type StageInstanceDelete struct {
	eventBase `json:"-"`
	StageInstance
}

func (StageInstanceDelete) eventName() string { return "STAGE_INSTANCE_DELETE" }

// PresenceUpdate fires when someone's status or activity changes. It needs the
// privileged GuildPresences intent, and on a large guild it is the noisiest
// event Discord sends - which is precisely the case Starlings' skip path is
// built for.
type PresenceUpdate struct {
	eventBase    `json:"-"`
	User         *User      `json:"user"`
	GuildID      Snowflake  `json:"guild_id"`
	Status       string     `json:"status"` // online, idle, dnd, offline
	Activities   []Activity `json:"activities"`
	ClientStatus struct {
		Desktop string `json:"desktop"`
		Mobile  string `json:"mobile"`
		Web     string `json:"web"`
	} `json:"client_status"`
	BeforeUpdate *PresenceUpdate `json:"-"`
}

func (PresenceUpdate) eventName() string { return "PRESENCE_UPDATE" }

// PresencesReplace is the legacy full presence snapshot event. Discord's
// current bot gateway sends incremental PresenceUpdate events, but older
// gateway versions and compatible implementations may still send this event.
type PresencesReplace []PresenceUpdate

func (PresencesReplace) eventName() string { return "PRESENCES_REPLACE" }

// UserUpdate fires when the bot's own user changes.
type UserUpdate struct {
	eventBase `json:"-"`
	User
	BeforeUpdate *User `json:"-"`
}

func (UserUpdate) eventName() string { return "USER_UPDATE" }

// Polls

// MessagePollVoteAdd fires when someone votes in a poll.
type MessagePollVoteAdd struct {
	eventBase `json:"-"`
	UserID    Snowflake `json:"user_id"`
	ChannelID Snowflake `json:"channel_id"`
	MessageID Snowflake `json:"message_id"`
	GuildID   Snowflake `json:"guild_id"`
	AnswerID  int       `json:"answer_id"`
}

func (MessagePollVoteAdd) eventName() string { return "MESSAGE_POLL_VOTE_ADD" }

// MessagePollVoteRemove fires when someone retracts a poll vote.
type MessagePollVoteRemove struct {
	eventBase `json:"-"`
	UserID    Snowflake `json:"user_id"`
	ChannelID Snowflake `json:"channel_id"`
	MessageID Snowflake `json:"message_id"`
	GuildID   Snowflake `json:"guild_id"`
	AnswerID  int       `json:"answer_id"`
}

func (MessagePollVoteRemove) eventName() string { return "MESSAGE_POLL_VOTE_REMOVE" }

// Voice

// VoiceChannelEffectSend fires when someone sends a reaction effect in a voice
// channel.
type VoiceChannelEffectSend struct {
	eventBase     `json:"-"`
	ChannelID     Snowflake `json:"channel_id"`
	GuildID       Snowflake `json:"guild_id"`
	UserID        Snowflake `json:"user_id"`
	Emoji         *Emoji    `json:"emoji"`
	AnimationType int       `json:"animation_type"`
	AnimationID   int       `json:"animation_id"`
	SoundID       Snowflake `json:"sound_id"`
	SoundVolume   float64   `json:"sound_volume"`
}

func (VoiceChannelEffectSend) eventName() string { return "VOICE_CHANNEL_EFFECT_SEND" }

// Monetisation

// Entitlement is a user's or guild's access to a premium SKU.
type Entitlement struct {
	ID                Snowflake       `json:"id"`
	SKUID             Snowflake       `json:"sku_id"`
	ApplicationID     Snowflake       `json:"application_id"`
	UserID            Snowflake       `json:"user_id"`
	GuildID           Snowflake       `json:"guild_id"`
	Type              EntitlementType `json:"type"`
	Deleted           bool            `json:"deleted"`
	StartsAt          *time.Time      `json:"starts_at"`
	EndsAt            *time.Time      `json:"ends_at"`
	Consumed          bool            `json:"consumed"`
	SubscriptionID    Snowflake       `json:"subscription_id"`
	GifterUserID      Snowflake       `json:"gifter_user_id"`
	ParentID          Snowflake       `json:"parent_id"`
	FulfilledAt       *time.Time      `json:"fulfilled_at"`
	FulfillmentStatus int             `json:"fulfillment_status"`
}

// EntitlementCreate fires when someone buys or is granted a SKU.
type EntitlementCreate struct {
	eventBase `json:"-"`
	Entitlement
}

func (EntitlementCreate) eventName() string { return "ENTITLEMENT_CREATE" }

// EntitlementUpdate fires when an entitlement changes, such as a renewal.
type EntitlementUpdate struct {
	eventBase `json:"-"`
	Entitlement
}

func (EntitlementUpdate) eventName() string { return "ENTITLEMENT_UPDATE" }

// EntitlementDelete fires when an entitlement is revoked. Note that an expiry
// is an update rather than a delete.
type EntitlementDelete struct {
	eventBase `json:"-"`
	Entitlement
}

func (EntitlementDelete) eventName() string { return "ENTITLEMENT_DELETE" }

// SubscriptionCreate fires when an application subscription starts.
type SubscriptionCreate struct {
	eventBase `json:"-"`
	Subscription
}

func (SubscriptionCreate) eventName() string { return "SUBSCRIPTION_CREATE" }

// SubscriptionUpdate fires when a subscription renews or changes state.
type SubscriptionUpdate struct {
	eventBase `json:"-"`
	Subscription
}

func (SubscriptionUpdate) eventName() string { return "SUBSCRIPTION_UPDATE" }

// SubscriptionDelete fires when a subscription ends.
type SubscriptionDelete struct {
	eventBase `json:"-"`
	Subscription
}

func (SubscriptionDelete) eventName() string { return "SUBSCRIPTION_DELETE" }

// Applications

// ApplicationCommandPermissionsUpdate fires when a guild changes who may use
// one of the application's commands.
type ApplicationCommandPermissionsUpdate struct {
	eventBase     `json:"-"`
	ID            Snowflake `json:"id"`
	ApplicationID Snowflake `json:"application_id"`
	GuildID       Snowflake `json:"guild_id"`
	Permissions   []struct {
		ID         Snowflake `json:"id"`
		Type       int       `json:"type"`
		Permission bool      `json:"permission"`
	} `json:"permissions"`
}

func (ApplicationCommandPermissionsUpdate) eventName() string {
	return "APPLICATION_COMMAND_PERMISSIONS_UPDATE"
}
