package starlings

import "time"

// MessageType marks how a message came to exist. Ordinary user and bot posts
// are MessageDefault or MessageReply; the rest are system notices such as join
// messages and pins.
type MessageType int

const (
	MessageDefault                                 MessageType = 0
	MessageRecipientAdd                            MessageType = 1
	MessageRecipientRemove                         MessageType = 2
	MessageCall                                    MessageType = 3
	MessageChannelNameChange                       MessageType = 4
	MessageChannelIconChange                       MessageType = 5
	MessageChannelPinnedMessage                    MessageType = 6
	MessageUserJoin                                MessageType = 7
	MessageGuildBoost                              MessageType = 8
	MessageGuildBoostTier1                         MessageType = 9
	MessageGuildBoostTier2                         MessageType = 10
	MessageGuildBoostTier3                         MessageType = 11
	MessageChannelFollowAdd                        MessageType = 12
	MessageGuildDiscoveryDisqualified              MessageType = 14
	MessageGuildDiscoveryRequalified               MessageType = 15
	MessageGuildDiscoveryGracePeriodInitialWarning MessageType = 16
	MessageGuildDiscoveryGracePeriodFinalWarning   MessageType = 17
	MessageThreadCreated                           MessageType = 18
	MessageReply                                   MessageType = 19
	MessageChatInputCommand                        MessageType = 20
	MessageThreadStarterMessage                    MessageType = 21
	MessageGuildInviteReminder                     MessageType = 22
	MessageContextMenuCommand                      MessageType = 23
	MessageAutoModerationAction                    MessageType = 24
	MessageRoleSubscriptionPurchase                MessageType = 25
	MessageInteractionPremiumUpsell                MessageType = 26
	MessageStageStart                              MessageType = 27
	MessageStageEnd                                MessageType = 28
	MessageStageSpeaker                            MessageType = 29
	MessageStageTopic                              MessageType = 30
	MessageGuildApplicationPremiumSubscription     MessageType = 31
	MessagePrivateChannelIntegrationAdded          MessageType = 32
	MessagePollResult                              MessageType = 36
)

// Message is a single post in a channel.
type Message struct {
	ID                   Snowflake                   `json:"id"`
	ChannelID            Snowflake                   `json:"channel_id"`
	GuildID              Snowflake                   `json:"guild_id"`
	Author               *User                       `json:"author"`
	Member               *Member                     `json:"member"` // the author's guild-specific data, guilds only
	Content              string                      `json:"content"`
	Timestamp            time.Time                   `json:"timestamp"`
	EditedTimestamp      *time.Time                  `json:"edited_timestamp"`
	TTS                  bool                        `json:"tts"`
	MentionEveryone      bool                        `json:"mention_everyone"`
	Mentions             []User                      `json:"mentions"`
	MentionRoles         []Snowflake                 `json:"mention_roles"`
	MentionChannels      []ChannelMention            `json:"mention_channels"`
	Attachments          []Attachment                `json:"attachments"`
	Embeds               []Embed                     `json:"embeds"`
	Reactions            []Reaction                  `json:"reactions"`
	Pinned               bool                        `json:"pinned"`
	WebhookID            Snowflake                   `json:"webhook_id"`
	Type                 MessageType                 `json:"type"`
	Flags                MessageFlags                `json:"flags"`
	ReferencedMsg        *Message                    `json:"referenced_message"`
	MessageRef           *MessageRef                 `json:"message_reference"`
	Components           []Component                 `json:"components"`
	Activity             *MessageActivity            `json:"activity"`
	Application          *MessageApplication         `json:"application"`
	ApplicationID        Snowflake                   `json:"application_id"`
	Interaction          *MessageInteraction         `json:"interaction"` // deprecated by Discord; kept for old messages
	InteractionMeta      *MessageInteractionMetadata `json:"interaction_metadata"`
	Thread               *Channel                    `json:"thread"`
	StickerItems         []StickerItem               `json:"sticker_items"`
	Position             int                         `json:"position"`
	RoleSubscription     *RoleSubscriptionData       `json:"role_subscription_data"`
	Resolved             *ResolvedData               `json:"resolved"`
	Poll                 *Poll                       `json:"poll"`
	Call                 *MessageCallInfo            `json:"call"`
	Snapshots            []MessageSnapshot           `json:"message_snapshots"`
	SharedClientTheme    *SharedClientTheme          `json:"shared_client_theme"`
	Nonce                any                         `json:"nonce"`
	Stickers             []Sticker                   `json:"stickers"` // legacy full sticker objects
	PurchaseNotification *PurchaseNotification       `json:"purchase_notification"`
	LobbyMember          *MessageLobbyMember         `json:"lobby_member"`
}

// MessageFlags is a bitmask of per-message options.
type MessageFlags int

const (
	MessageFlagCrossposted MessageFlags = 1 << iota
	MessageFlagIsCrosspost
	MessageFlagSuppressEmbeds
	MessageFlagSourceMessageDeleted
	MessageFlagUrgent
	MessageFlagHasThread
	MessageFlagEphemeral // only the invoking user can see it
	MessageFlagLoading   // the "thinking..." state of a deferred interaction
	MessageFlagFailedToMentionSomeRolesInThread
	_
	_
	_
	MessageFlagSuppressNotifications
	MessageFlagIsVoiceMessage
	_ // bit 14 is not currently assigned by Discord
	MessageFlagIsComponentsV2
)

// MessageReferenceType distinguishes a reply from a forwarded-message
// snapshot.
type MessageReferenceType int

const (
	MessageReferenceDefault MessageReferenceType = iota
	MessageReferenceForward
)

// MessageRef points at another message, used for replies and forwards.
type MessageRef struct {
	Type      MessageReferenceType `json:"type,omitempty"`
	MessageID Snowflake            `json:"message_id"`
	ChannelID Snowflake            `json:"channel_id,omitempty"`
	GuildID   Snowflake            `json:"guild_id,omitempty"`
	// FailIfNotExists makes sending fail when the referenced message is gone.
	// Discord's default is true; the pointer lets you send an explicit false.
	FailIfNotExists *bool `json:"fail_if_not_exists,omitempty"`
}

// ChannelMention is the small channel shape embedded in crossposted messages.
type ChannelMention struct {
	ID      Snowflake   `json:"id"`
	GuildID Snowflake   `json:"guild_id"`
	Type    ChannelType `json:"type"`
	Name    string      `json:"name"`
}

type MessageActivityType int

const (
	MessageActivityJoin        MessageActivityType = 1
	MessageActivitySpectate    MessageActivityType = 2
	MessageActivityListen      MessageActivityType = 3
	MessageActivityJoinRequest MessageActivityType = 5
)

type MessageActivity struct {
	Type    MessageActivityType `json:"type"`
	PartyID string              `json:"party_id,omitempty"`
}

type MessageApplication struct {
	ID           Snowflake `json:"id"`
	CoverImage   string    `json:"cover_image,omitempty"`
	Description  string    `json:"description"`
	Icon         string    `json:"icon,omitempty"`
	Name         string    `json:"name"`
	Bot          *User     `json:"bot"`
	Flags        int       `json:"flags"`
	FlagsNew     string    `json:"flags_new"`
	PrimarySKUID Snowflake `json:"primary_sku_id"`
	Type         int       `json:"type"`
}

type MessageSnapshot struct {
	Message *Message `json:"message"`
}

type StickerItem struct {
	ID         Snowflake `json:"id"`
	Name       string    `json:"name"`
	FormatType int       `json:"format_type"`
}

type MessageInteraction struct {
	ID     Snowflake       `json:"id"`
	Type   InteractionType `json:"type"`
	Name   string          `json:"name"`
	User   *User           `json:"user"`
	Member *Member         `json:"member"`
}

type MessageInteractionMetadata struct {
	ID                            Snowflake                   `json:"id"`
	Type                          InteractionType             `json:"type"`
	User                          *User                       `json:"user"`
	AuthorizingIntegrationOwners  map[string]Snowflake        `json:"authorizing_integration_owners"`
	OriginalResponseMessageID     Snowflake                   `json:"original_response_message_id"`
	InteractedMessageID           Snowflake                   `json:"interacted_message_id"`
	TargetMessageID               Snowflake                   `json:"target_message_id"`
	TargetUser                    *User                       `json:"target_user"`
	TriggeringInteractionMetadata *MessageInteractionMetadata `json:"triggering_interaction_metadata"`
}

type PurchaseNotification struct {
	Type                 int                   `json:"type"`
	GuildProductPurchase *GuildProductPurchase `json:"guild_product_purchase"`
}

type GuildProductPurchase struct {
	ListingID   Snowflake `json:"listing_id"`
	ProductName string    `json:"product_name"`
}

type MessageLobbyMember struct {
	AdditionalName string `json:"additional_name"`
}

type RoleSubscriptionData struct {
	RoleSubscriptionListingID Snowflake `json:"role_subscription_listing_id"`
	TierName                  string    `json:"tier_name"`
	TotalMonthsSubscribed     int       `json:"total_months_subscribed"`
	IsRenewal                 bool      `json:"is_renewal"`
}

type PollLayoutType int

const PollLayoutDefault PollLayoutType = 1

type PollMedia struct {
	Text  string `json:"text,omitempty"`
	Emoji *Emoji `json:"emoji,omitempty"`
}

type PollAnswer struct {
	AnswerID int       `json:"answer_id,omitempty"`
	Media    PollMedia `json:"poll_media"`
}

type PollAnswerCount struct {
	ID      int  `json:"id"`
	Count   int  `json:"count"`
	MeVoted bool `json:"me_voted"`
}

type PollResults struct {
	Finalized    bool              `json:"is_finalized"`
	AnswerCounts []PollAnswerCount `json:"answer_counts"`
}

// Poll is accepted when sending and fully populated when receiving. Duration
// is the requested number of hours; Expiry and Results are response fields.
type Poll struct {
	Question         PollMedia      `json:"question"`
	Answers          []PollAnswer   `json:"answers"`
	Expiry           *time.Time     `json:"expiry,omitempty"`
	AllowMultiselect bool           `json:"allow_multiselect,omitempty"`
	LayoutType       PollLayoutType `json:"layout_type"`
	Results          *PollResults   `json:"results,omitempty"`
	Duration         int            `json:"duration,omitempty"`
}

// NewPoll builds the sendable part of a poll with Discord's default layout.
func NewPoll(question string, answers ...string) Poll {
	p := Poll{Question: PollMedia{Text: question}, LayoutType: PollLayoutDefault}
	p.Answers = make([]PollAnswer, len(answers))
	for i, answer := range answers {
		p.Answers[i].Media.Text = answer
	}
	return p
}

type MessageCallInfo struct {
	Participants   []Snowflake `json:"participants"`
	EndedTimestamp *time.Time  `json:"ended_timestamp"`
}

type SharedClientTheme struct {
	BackgroundGradientPresetID *int `json:"background_gradient_preset_id,omitempty"`
	BackgroundGradientAngle    *int `json:"background_gradient_angle,omitempty"`
}

// Attachment is a file uploaded alongside a message.
type Attachment struct {
	ID                 Snowflake           `json:"id"`
	Filename           string              `json:"filename"`
	Description        string              `json:"description"`
	ContentType        string              `json:"content_type"`
	Size               int                 `json:"size"`
	URL                string              `json:"url"`
	ProxyURL           string              `json:"proxy_url"`
	Height             int                 `json:"height"`
	Width              int                 `json:"width"`
	Ephemeral          bool                `json:"ephemeral"`
	Title              string              `json:"title"`
	DurationSecs       float64             `json:"duration_secs"`
	Waveform           string              `json:"waveform"`
	Flags              int                 `json:"flags"`
	Application        *MessageApplication `json:"application"`
	ClipCreatedAt      *time.Time          `json:"clip_created_at"`
	ClipParticipants   []User              `json:"clip_participants"`
	Placeholder        string              `json:"placeholder"`
	PlaceholderVersion int                 `json:"placeholder_version"`
}

// Reaction is the aggregate count of one emoji on a message.
type Reaction struct {
	Count        int                  `json:"count"`
	CountDetails ReactionCountDetails `json:"count_details"`
	Me           bool                 `json:"me"` // whether the current bot reacted
	MeBurst      bool                 `json:"me_burst"`
	Emoji        Emoji                `json:"emoji"`
	BurstColors  []string             `json:"burst_colors"`
}

type ReactionCountDetails struct {
	Burst  int `json:"burst"`
	Normal int `json:"normal"`
}

// Emoji is either a Unicode emoji (ID zero, Name holds the character) or a
// custom guild emoji (ID set).
type Emoji struct {
	ID       Snowflake `json:"id"`
	Name     string    `json:"name"`
	Animated bool      `json:"animated"`
}

// APIFormat returns the form Discord's reaction endpoints expect: the raw
// character for a Unicode emoji, or name:id for a custom one.
func (e Emoji) APIFormat() string {
	if e.ID.IsZero() {
		return e.Name
	}
	return e.Name + ":" + e.ID.String()
}
