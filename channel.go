package starlings

import "time"

// ChannelType distinguishes the kinds of channel Discord models with one type.
type ChannelType int

const (
	ChannelGuildText ChannelType = iota
	ChannelDM
	ChannelGuildVoice
	ChannelGroupDM
	ChannelGuildCategory
	ChannelGuildAnnouncement
	_
	_
	_
	_
	ChannelAnnouncementThread
	ChannelPublicThread
	ChannelPrivateThread
	ChannelGuildStageVoice
	ChannelGuildDirectory
	ChannelGuildForum
	ChannelGuildMedia
)

// IsThread reports whether the type is one of the three thread kinds.
func (t ChannelType) IsThread() bool {
	return t == ChannelAnnouncementThread || t == ChannelPublicThread || t == ChannelPrivateThread
}

// Channel is any place messages can live: a guild text channel, a voice
// channel, a DM, a category, a thread or a forum. Which fields are populated
// depends on Type.
type Channel struct {
	ID                   Snowflake      `json:"id"`
	Type                 ChannelType    `json:"type"`
	GuildID              Snowflake      `json:"guild_id"`
	Position             int            `json:"position"`
	PermissionOverwrites []Overwrite    `json:"permission_overwrites"`
	Name                 string         `json:"name"`
	Topic                string         `json:"topic"`
	NSFW                 bool           `json:"nsfw"`
	LastMessageID        Snowflake      `json:"last_message_id"`
	Bitrate              int            `json:"bitrate"`
	UserLimit            int            `json:"user_limit"`
	RateLimitPerUser     int            `json:"rate_limit_per_user"` // slow mode, in seconds
	Recipients           []User         `json:"recipients"`          // DM and group DM only
	OwnerID              Snowflake      `json:"owner_id"`
	ParentID             Snowflake      `json:"parent_id"` // category, or the parent of a thread
	LastPinTimestamp     *time.Time     `json:"last_pin_timestamp"`
	Flags                int            `json:"flags"`
	ThreadMetadata       *ThreadMeta    `json:"thread_metadata"`
	AvailableTags        []ForumTag     `json:"available_tags"`
	AppliedTags          []Snowflake    `json:"applied_tags"`
	DefaultReaction      *ForumReaction `json:"default_reaction_emoji"`
	Status               string         `json:"status"`
	VoiceStartTime       *time.Time     `json:"voice_start_time"`

	ref bound `json:"-"` // the client this value came from
}

// Mention returns the <#id> form that renders as a channel link.
func (c *Channel) Mention() string { return "<#" + c.ID.String() + ">" }

// Overwrite is a per-channel permission adjustment for one role or member.
// Deny is applied before Allow.
type Overwrite struct {
	ID    Snowflake   `json:"id"`
	Type  int         `json:"type"` // 0 = role, 1 = member
	Allow Permissions `json:"allow,string"`
	Deny  Permissions `json:"deny,string"`
}

// ThreadMeta holds the fields that only exist on threads.
type ThreadMeta struct {
	Archived            bool       `json:"archived"`
	AutoArchiveDuration int        `json:"auto_archive_duration"` // minutes
	ArchiveTimestamp    time.Time  `json:"archive_timestamp"`
	Locked              bool       `json:"locked"`
	Invitable           bool       `json:"invitable"`
	CreateTimestamp     *time.Time `json:"create_timestamp"`
}

// ForumTag is a selectable tag on a forum or media channel.
type ForumTag struct {
	ID        Snowflake `json:"id"`
	Name      string    `json:"name"`
	Moderated bool      `json:"moderated"`
	EmojiID   Snowflake `json:"emoji_id"`
	EmojiName string    `json:"emoji_name"`
}

// ForumReaction is the emoji shown as a forum channel's default reaction.
type ForumReaction struct {
	EmojiID   Snowflake `json:"emoji_id"`
	EmojiName string    `json:"emoji_name"`
}
