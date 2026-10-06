package starlings

import (
	"context"
	"net/http"
	"net/url"
)

// AutomodTriggerType says what an auto-moderation rule watches for.
const (
	TriggerKeyword       = 1 // custom words and patterns
	TriggerSpam          = 4 // Discord's own spam detection
	TriggerKeywordPreset = 5 // Discord's curated word lists
	TriggerMentionSpam   = 6 // too many mentions in one message
	TriggerMemberProfile = 7 // matches in usernames and bios
)

// AutomodEventType says when a rule is evaluated.
const (
	EventMessageSend  = 1
	EventMemberUpdate = 2
)

// AutomodActionType is what happens when a rule matches.
const (
	ActionBlockMessage     = 1
	ActionSendAlertMessage = 2
	ActionTimeout          = 3
	ActionBlockInteraction = 4
)

// Keyword preset lists Discord maintains.
const (
	PresetProfanity     = 1
	PresetSexualContent = 2
	PresetSlurs         = 3
)

// AutomodRule is one auto-moderation rule.
type AutomodRule struct {
	ID              Snowflake           `json:"id,omitzero"`
	GuildID         Snowflake           `json:"guild_id,omitzero"`
	Name            string              `json:"name"`
	CreatorID       Snowflake           `json:"creator_id,omitzero"`
	EventType       int                 `json:"event_type"`
	TriggerType     int                 `json:"trigger_type"`
	TriggerMetadata *AutomodTriggerMeta `json:"trigger_metadata,omitzero"`
	Actions         []AutomodAction     `json:"actions"`
	Enabled         bool                `json:"enabled"`
	ExemptRoles     []Snowflake         `json:"exempt_roles,omitzero"`
	ExemptChannels  []Snowflake         `json:"exempt_channels,omitzero"`
}

// AutomodTriggerMeta refines what a rule matches. Which fields apply depends
// on the trigger type.
type AutomodTriggerMeta struct {
	KeywordFilter                []string `json:"keyword_filter,omitzero"`
	RegexPatterns                []string `json:"regex_patterns,omitzero"`
	Presets                      []int    `json:"presets,omitzero"`
	AllowList                    []string `json:"allow_list,omitzero"`
	MentionTotalLimit            int      `json:"mention_total_limit,omitzero"`
	MentionRaidProtectionEnabled bool     `json:"mention_raid_protection_enabled,omitzero"`
}

// AutomodAction is one consequence of a rule matching.
type AutomodAction struct {
	Type     int                `json:"type"`
	Metadata *AutomodActionMeta `json:"metadata,omitzero"`
}

// AutomodActionMeta configures an action.
type AutomodActionMeta struct {
	ChannelID       Snowflake `json:"channel_id,omitzero"`       // where to send alerts
	DurationSeconds int       `json:"duration_seconds,omitzero"` // timeout length, max 4 weeks
	CustomMessage   string    `json:"custom_message,omitzero"`   // shown to the blocked user
}

// AutomodRules lists a guild's auto-moderation rules.
func (c *Client) AutomodRules(ctx context.Context, guildID Snowflake) ([]AutomodRule, error) {
	var out []AutomodRule
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/auto-moderation/rules",
		Route:  "GET /guilds/" + guildID.String() + "/auto-moderation/rules",
	}, &out)
	return out, err
}

// AutomodRule fetches one rule.
func (c *Client) AutomodRule(ctx context.Context, guildID, ruleID Snowflake) (*AutomodRule, error) {
	var out AutomodRule
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/auto-moderation/rules/" + ruleID.String(),
		Route:  "GET /guilds/" + guildID.String() + "/auto-moderation/rules/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateAutomodRule adds an auto-moderation rule.
//
// A guild may hold a limited number of rules per trigger type - currently six
// keyword rules and one each of spam, preset and mention-spam.
func (c *Client) CreateAutomodRule(ctx context.Context, guildID Snowflake, rule AutomodRule, reason string) (*AutomodRule, error) {
	var out AutomodRule
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/auto-moderation/rules",
		Route:  "POST /guilds/" + guildID.String() + "/auto-moderation/rules",
		Body:   rule,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyAutomodRule edits a rule.
func (c *Client) ModifyAutomodRule(ctx context.Context, guildID, ruleID Snowflake, rule AutomodRule, reason string) (*AutomodRule, error) {
	var out AutomodRule
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/auto-moderation/rules/" + ruleID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/auto-moderation/rules/{id}",
		Body:   rule,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAutomodRule removes a rule.
func (c *Client) DeleteAutomodRule(ctx context.Context, guildID, ruleID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/auto-moderation/rules/" + ruleID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/auto-moderation/rules/{id}",
		Reason: reason,
	}, nil)
}

// Stage instances

// StageInstance is a live stage channel event.
type StageInstance struct {
	ID                    Snowflake `json:"id,omitzero"`
	GuildID               Snowflake `json:"guild_id,omitzero"`
	ChannelID             Snowflake `json:"channel_id"`
	Topic                 string    `json:"topic"`
	PrivacyLevel          int       `json:"privacy_level,omitzero"` // 2 = guild only
	GuildScheduledEventID Snowflake `json:"guild_scheduled_event_id,omitzero"`
}

// CreateStageInstance opens a stage channel to listeners.
func (c *Client) CreateStageInstance(ctx context.Context, s StageInstance, reason string) (*StageInstance, error) {
	if s.PrivacyLevel == 0 {
		s.PrivacyLevel = 2
	}

	var out StageInstance
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/stage-instances",
		Route:  "POST /stage-instances",
		Body:   s,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// StageInstance fetches the live stage for a channel.
func (c *Client) StageInstance(ctx context.Context, channelID Snowflake) (*StageInstance, error) {
	var out StageInstance
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/stage-instances/" + channelID.String(),
		Route:  "GET /stage-instances/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyStageInstance changes a live stage's topic.
func (c *Client) ModifyStageInstance(ctx context.Context, channelID Snowflake, topic, reason string) (*StageInstance, error) {
	var out StageInstance
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/stage-instances/" + channelID.String(),
		Route:  "PATCH /stage-instances/{id}",
		Body:   map[string]string{"topic": topic},
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteStageInstance ends a live stage.
func (c *Client) DeleteStageInstance(ctx context.Context, channelID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/stage-instances/" + channelID.String(),
		Route:  "DELETE /stage-instances/{id}",
		Reason: reason,
	}, nil)
}

// Polls

// PollVotersQuery pages through the people who picked one poll answer.
type PollVotersQuery struct {
	After Snowflake
	Limit int
}

// PollAnswerVoters lists who voted for one answer on a poll.
func (c *Client) PollAnswerVoters(ctx context.Context, channelID, messageID Snowflake, answerID int, q PollVotersQuery) ([]User, error) {
	v := url.Values{}
	if !q.After.IsZero() {
		v.Set("after", q.After.String())
	}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}

	var out struct {
		Users []User `json:"users"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/channels/" + channelID.String() + "/polls/" + messageID.String() +
			"/answers/" + itoa(answerID) + query(v),
		Route: "GET /channels/" + channelID.String() + "/polls/{id}/answers/{id}",
	}, &out)
	return out.Users, err
}

// ExpirePoll ends a poll early, freezing the results.
func (c *Client) ExpirePoll(ctx context.Context, channelID, messageID Snowflake) (*Message, error) {
	var out Message
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/polls/" + messageID.String() + "/expire",
		Route:  "POST /channels/" + channelID.String() + "/polls/{id}/expire",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
