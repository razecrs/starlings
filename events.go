package starlings

import (
	"context"
	"time"
)

// Event is implemented by every gateway event type. The method is unexported,
// so the set of events is closed to this package - which is what lets On
// resolve a handler to a Discord event name with no configuration.
type Event interface {
	eventName() string
}

// eventBase carries the client back to the handler, so events can offer
// methods like MessageCreate.Reply instead of making you thread a client
// through yourself. The field is unexported and therefore invisible to the
// JSON decoder.
type eventBase struct{ c *Client }

func (e *eventBase) bind(c *Client) { e.c = c }

// Client returns the client that received the event.
func (e *eventBase) Client() *Client { return e.c }

// binder is satisfied by every event through eventBase.
type binder interface{ bind(*Client) }

// Lifecycle

// Connect fires after a gateway shard has completed its websocket handshake
// and sent Identify or Resume. It is a Starlings lifecycle event, not a Discord
// dispatch.
type Connect struct {
	eventBase `json:"-"`
	ShardID   int `json:"shard_id"`
}

func (Connect) eventName() string { return "CONNECT" }

// Disconnect fires when an established gateway shard closes, including when
// Starlings is about to reconnect it.
type Disconnect struct {
	eventBase `json:"-"`
	ShardID   int `json:"shard_id"`
	// CloseCode and CloseReason are set when Discord closed the connection.
	// They are zero for network failures and for a client shutting down.
	CloseCode   CloseCode `json:"close_code,omitzero"`
	CloseReason string    `json:"close_reason,omitzero"`
}

func (Disconnect) eventName() string { return "DISCONNECT" }

// RateLimit fires whenever Discord returns an HTTP 429. Starlings still waits
// and retries automatically; this event exists for metrics and alerting.
type RateLimit struct {
	eventBase         `json:"-"`
	Method            string  `json:"method"`
	Path              string  `json:"path"`
	Route             string  `json:"route"`
	RetryAfterSeconds float64 `json:"retry_after"`
	Global            bool    `json:"global"`
}

// Wait returns RetryAfterSeconds as a Go duration.
func (r RateLimit) Wait() time.Duration {
	return time.Duration(r.RetryAfterSeconds * float64(time.Second))
}

func (RateLimit) eventName() string { return "REST_RATE_LIMIT" }

// RateLimitWait fires when a REST request waited for a rate-limit bucket that
// Starlings already knew was empty. Unlike RateLimit, nothing was rejected;
// it shows where the bot spends time waiting on Discord.
type RateLimitWait struct {
	eventBase   `json:"-"`
	Method      string  `json:"method"`
	Route       string  `json:"route"`
	WaitSeconds float64 `json:"wait_seconds"`
}

// Wait returns WaitSeconds as a Go duration.
func (r RateLimitWait) Wait() time.Duration {
	return time.Duration(r.WaitSeconds * float64(time.Second))
}

func (RateLimitWait) eventName() string { return "REST_RATE_LIMIT_WAIT" }

// Ready fires once the gateway has accepted the connection and sent the
// initial state. It is the signal that the bot is online.
type Ready struct {
	eventBase        `json:"-"`
	Version          int      `json:"v"`
	User             *User    `json:"user"`
	Guilds           []Guild  `json:"guilds"` // unavailable stubs; GuildCreate follows for each
	SessionID        string   `json:"session_id"`
	ResumeGatewayURL string   `json:"resume_gateway_url"`
	Application      *AppInfo `json:"application"`
	Shard            [2]int   `json:"shard"`
}

func (Ready) eventName() string { return "READY" }

// AppInfo is the slice of application data Discord includes in Ready.
type AppInfo struct {
	ID    Snowflake `json:"id"`
	Flags int       `json:"flags"`
}

// Resumed fires when a dropped connection has been resumed and any events
// missed in the gap have been replayed.
type Resumed struct {
	eventBase `json:"-"`
}

func (Resumed) eventName() string { return "RESUMED" }

// RateLimited fires when Discord rejects a gateway request, currently member
// chunk requests. RetryAfter is in seconds.
type RateLimited struct {
	eventBase  `json:"-"`
	Opcode     Opcode  `json:"opcode"`
	RetryAfter float64 `json:"retry_after"`
	Meta       struct {
		GuildID Snowflake `json:"guild_id"`
		Nonce   string    `json:"nonce"`
	} `json:"meta"`
}

func (RateLimited) eventName() string { return "RATE_LIMITED" }

// Messages

// MessageCreate fires when a message is posted in a channel the bot can see.
//
// Content is empty unless the bot has the privileged MessageContent intent,
// or the message mentions the bot, or it is a DM.
type MessageCreate struct {
	eventBase `json:"-"`
	Message
}

func (MessageCreate) eventName() string { return "MESSAGE_CREATE" }

// MessageUpdate fires when a message is edited. Discord sends a partial
// message, so fields the edit did not touch may be zero.
type MessageUpdate struct {
	eventBase `json:"-"`
	Message
	BeforeUpdate *Message `json:"-"`
}

func (MessageUpdate) eventName() string { return "MESSAGE_UPDATE" }

// MessageDelete fires when a single message is deleted. Only the IDs are sent -
// the content is gone.
type MessageDelete struct {
	eventBase    `json:"-"`
	ID           Snowflake `json:"id"`
	ChannelID    Snowflake `json:"channel_id"`
	GuildID      Snowflake `json:"guild_id"`
	BeforeDelete *Message  `json:"-"`
}

func (MessageDelete) eventName() string { return "MESSAGE_DELETE" }

// MessageDeleteBulk fires when messages are purged together.
type MessageDeleteBulk struct {
	eventBase    `json:"-"`
	IDs          []Snowflake `json:"ids"`
	ChannelID    Snowflake   `json:"channel_id"`
	GuildID      Snowflake   `json:"guild_id"`
	BeforeDelete []Message   `json:"-"`
}

func (MessageDeleteBulk) eventName() string { return "MESSAGE_DELETE_BULK" }

// Reactions

// MessageReactionAdd fires when someone reacts to a message.
type MessageReactionAdd struct {
	eventBase    `json:"-"`
	UserID       Snowflake `json:"user_id"`
	ChannelID    Snowflake `json:"channel_id"`
	MessageID    Snowflake `json:"message_id"`
	GuildID      Snowflake `json:"guild_id"`
	Member       *Member   `json:"member"`
	Emoji        Emoji     `json:"emoji"`
	BeforeUpdate *Message  `json:"-"`
}

func (MessageReactionAdd) eventName() string { return "MESSAGE_REACTION_ADD" }

// MessageReactionRemove fires when someone takes a reaction back.
type MessageReactionRemove struct {
	eventBase    `json:"-"`
	UserID       Snowflake `json:"user_id"`
	ChannelID    Snowflake `json:"channel_id"`
	MessageID    Snowflake `json:"message_id"`
	GuildID      Snowflake `json:"guild_id"`
	Emoji        Emoji     `json:"emoji"`
	BeforeUpdate *Message  `json:"-"`
}

func (MessageReactionRemove) eventName() string { return "MESSAGE_REACTION_REMOVE" }

// MessageReactionRemoveAll fires when every reaction is cleared from a message.
type MessageReactionRemoveAll struct {
	eventBase    `json:"-"`
	ChannelID    Snowflake `json:"channel_id"`
	MessageID    Snowflake `json:"message_id"`
	GuildID      Snowflake `json:"guild_id"`
	BeforeUpdate *Message  `json:"-"`
}

func (MessageReactionRemoveAll) eventName() string { return "MESSAGE_REACTION_REMOVE_ALL" }

// Guilds

// GuildCreate fires on connect for every guild the bot is in, and again
// whenever it joins a new one.
type GuildCreate struct {
	eventBase `json:"-"`
	Guild
}

func (GuildCreate) eventName() string { return "GUILD_CREATE" }

// GuildUpdate fires when a guild's settings change.
type GuildUpdate struct {
	eventBase `json:"-"`
	Guild
	BeforeUpdate *Guild `json:"-"`
}

func (GuildUpdate) eventName() string { return "GUILD_UPDATE" }

// GuildDelete fires when the bot leaves a guild, or the guild goes offline.
// Unavailable distinguishes the two.
type GuildDelete struct {
	eventBase    `json:"-"`
	ID           Snowflake `json:"id"`
	Unavailable  bool      `json:"unavailable"` // true means an outage, not a removal
	BeforeDelete *Guild    `json:"-"`
}

func (GuildDelete) eventName() string { return "GUILD_DELETE" }

// GuildMemberAdd fires when someone joins a guild. Needs the privileged
// GuildMembers intent.
type GuildMemberAdd struct {
	eventBase `json:"-"`
	Member
	GuildID Snowflake `json:"guild_id"`
}

func (GuildMemberAdd) eventName() string { return "GUILD_MEMBER_ADD" }

// GuildMemberUpdate fires when a member's roles, nickname or timeout change.
//
// Discord sends the member's complete current state, so a null field means
// the value was removed: CommunicationDisabledUntil is nil once a timeout is
// lifted, and Avatar is empty once a guild avatar is reset.
type GuildMemberUpdate struct {
	eventBase                  `json:"-"`
	GuildID                    Snowflake   `json:"guild_id"`
	User                       *User       `json:"user"`
	Nick                       string      `json:"nick"`
	Avatar                     string      `json:"avatar"`
	Roles                      []Snowflake `json:"roles"`
	JoinedAt                   *time.Time  `json:"joined_at"`
	PremiumSince               *time.Time  `json:"premium_since"`
	Deaf                       *bool       `json:"deaf"`
	Mute                       *bool       `json:"mute"`
	Pending                    bool        `json:"pending"`
	Flags                      int         `json:"flags"`
	CommunicationDisabledUntil *time.Time  `json:"communication_disabled_until"`
	BeforeUpdate               *Member     `json:"-"`
}

func (GuildMemberUpdate) eventName() string { return "GUILD_MEMBER_UPDATE" }

// GuildMemberRemove fires when someone leaves or is removed from a guild.
type GuildMemberRemove struct {
	eventBase    `json:"-"`
	GuildID      Snowflake `json:"guild_id"`
	User         *User     `json:"user"`
	BeforeDelete *Member   `json:"-"`
}

func (GuildMemberRemove) eventName() string { return "GUILD_MEMBER_REMOVE" }

// Channels

// ChannelCreate fires when a channel is created.
type ChannelCreate struct {
	eventBase `json:"-"`
	Channel
}

func (ChannelCreate) eventName() string { return "CHANNEL_CREATE" }

// ChannelUpdate fires when a channel's settings change.
type ChannelUpdate struct {
	eventBase `json:"-"`
	Channel
	BeforeUpdate *Channel `json:"-"`
}

func (ChannelUpdate) eventName() string { return "CHANNEL_UPDATE" }

// ChannelInfo answers RequestChannelInfo with ephemeral voice-channel data.
type ChannelInfo struct {
	eventBase `json:"-"`
	GuildID   Snowflake            `json:"guild_id"`
	Channels  []ChannelInfoChannel `json:"channels"`
}

func (ChannelInfo) eventName() string { return "CHANNEL_INFO" }

// ChannelInfoChannel is the ephemeral subset returned by ChannelInfo.
// VoiceStartTime is a Unix timestamp in seconds.
type ChannelInfoChannel struct {
	ID             Snowflake `json:"id"`
	Status         *string   `json:"status"`
	VoiceStartTime *int64    `json:"voice_start_time"`
}

// VoiceChannelStatusUpdate fires when a voice channel's visible status changes.
type VoiceChannelStatusUpdate struct {
	eventBase    `json:"-"`
	ID           Snowflake `json:"id"`
	GuildID      Snowflake `json:"guild_id"`
	Status       *string   `json:"status"`
	BeforeUpdate *Channel  `json:"-"`
}

func (VoiceChannelStatusUpdate) eventName() string { return "VOICE_CHANNEL_STATUS_UPDATE" }

// VoiceChannelStartTimeUpdate fires when a voice session starts or ends.
type VoiceChannelStartTimeUpdate struct {
	eventBase      `json:"-"`
	ID             Snowflake `json:"id"`
	GuildID        Snowflake `json:"guild_id"`
	VoiceStartTime *int64    `json:"voice_start_time"`
	BeforeUpdate   *Channel  `json:"-"`
}

func (VoiceChannelStartTimeUpdate) eventName() string {
	return "VOICE_CHANNEL_START_TIME_UPDATE"
}

// ChannelDelete fires when a channel is deleted.
type ChannelDelete struct {
	eventBase `json:"-"`
	Channel
	BeforeDelete *Channel `json:"-"`
}

func (ChannelDelete) eventName() string { return "CHANNEL_DELETE" }

// Interactions

// InteractionCreate fires when someone uses a slash command, presses a button
// or submits a modal. Discord expects a response within three seconds.
type InteractionCreate struct {
	eventBase `json:"-"`
	Interaction

	// answered guards the single initial response Discord permits.
	//
	// A raw int32 rather than atomic.Bool: the latter embeds noCopy, and
	// eventName has a value receiver, so it would make the whole event type
	// un-copyable and fail vet.
	answered int32 `json:"-"`

	// auto is set when Starlings will defer the interaction if the handler
	// has not answered in time.
	auto *autoDeferState `json:"-"`

	// finished is set once a deferred response has been edited, deleted, or
	// followed up, so failure reporting knows whether "thinking..." remains.
	finished int32 `json:"-"`

	// params holds values captured by a custom-ID route with {parameters}.
	params map[string]string `json:"-"`

	// respondHTTP is set only for interactions received through an HTTP
	// endpoint. Gateway interactions leave it nil and use the REST callback.
	respondHTTP func(context.Context, InteractionResponse, []File) error
}

func (InteractionCreate) eventName() string { return "INTERACTION_CREATE" }

// Presence

// TypingStart fires when a user starts typing in a channel.
type TypingStart struct {
	eventBase `json:"-"`
	ChannelID Snowflake `json:"channel_id"`
	GuildID   Snowflake `json:"guild_id"`
	UserID    Snowflake `json:"user_id"`
	Timestamp int64     `json:"timestamp"` // unix seconds
	Member    *Member   `json:"member"`
}

func (TypingStart) eventName() string { return "TYPING_START" }

// VoiceStateUpdate fires when someone joins, leaves or moves between voice
// channels, or mutes themselves. It needs the GuildVoiceStates intent, which
// is not privileged.
//
// ChannelID is zero when the user has disconnected.
type VoiceStateUpdate struct {
	eventBase `json:"-"`
	VoiceState
	BeforeUpdate *VoiceState `json:"-"`
}

func (VoiceStateUpdate) eventName() string { return "VOICE_STATE_UPDATE" }

// VoiceServerUpdate carries the endpoint and token used by ConnectVoice.
type VoiceServerUpdate struct {
	eventBase `json:"-"`
	Token     string    `json:"token"`
	GuildID   Snowflake `json:"guild_id"`
	Endpoint  string    `json:"endpoint"`
}

func (VoiceServerUpdate) eventName() string { return "VOICE_SERVER_UPDATE" }

// MessageReactionRemoveEmoji fires when every reaction of one emoji is cleared
// from a message.
type MessageReactionRemoveEmoji struct {
	eventBase    `json:"-"`
	ChannelID    Snowflake `json:"channel_id"`
	GuildID      Snowflake `json:"guild_id"`
	MessageID    Snowflake `json:"message_id"`
	Emoji        Emoji     `json:"emoji"`
	BeforeUpdate *Message  `json:"-"`
}

func (MessageReactionRemoveEmoji) eventName() string { return "MESSAGE_REACTION_REMOVE_EMOJI" }
