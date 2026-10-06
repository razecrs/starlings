package starlings

import "strings"

// eventIntents maps a gateway event to the intents that can deliver it. An
// event is delivered if *any* listed intent is enabled, because several events
// arrive under either a guild intent or its direct-message twin.
//
// Transcribed from docs/gateway-events.md.
var eventIntents = map[string][]Intent{
	"MESSAGE_CREATE":                    {IntentGuildMessages, IntentDirectMessages},
	"MESSAGE_UPDATE":                    {IntentGuildMessages, IntentDirectMessages},
	"MESSAGE_DELETE":                    {IntentGuildMessages, IntentDirectMessages},
	"MESSAGE_DELETE_BULK":               {IntentGuildMessages},
	"MESSAGE_REACTION_ADD":              {IntentGuildMessageReactions, IntentDirectMessageReactions},
	"MESSAGE_REACTION_REMOVE":           {IntentGuildMessageReactions, IntentDirectMessageReactions},
	"MESSAGE_REACTION_REMOVE_ALL":       {IntentGuildMessageReactions, IntentDirectMessageReactions},
	"TYPING_START":                      {IntentGuildMessageTyping, IntentDirectMessageTyping},
	"GUILD_CREATE":                      {IntentGuilds},
	"GUILD_UPDATE":                      {IntentGuilds},
	"GUILD_DELETE":                      {IntentGuilds},
	"CHANNEL_CREATE":                    {IntentGuilds},
	"CHANNEL_UPDATE":                    {IntentGuilds},
	"CHANNEL_DELETE":                    {IntentGuilds},
	"GUILD_MEMBER_ADD":                  {IntentGuildMembers},
	"GUILD_MEMBER_UPDATE":               {IntentGuildMembers},
	"GUILD_MEMBER_REMOVE":               {IntentGuildMembers},
	"VOICE_STATE_UPDATE":                {IntentGuildVoiceStates},
	"CHANNEL_PINS_UPDATE":               {IntentGuilds, IntentDirectMessages},
	"THREAD_CREATE":                     {IntentGuilds},
	"THREAD_UPDATE":                     {IntentGuilds},
	"THREAD_DELETE":                     {IntentGuilds},
	"THREAD_LIST_SYNC":                  {IntentGuilds},
	"THREAD_MEMBER_UPDATE":              {IntentGuilds},
	"THREAD_MEMBERS_UPDATE":             {IntentGuilds, IntentGuildMembers},
	"STAGE_INSTANCE_CREATE":             {IntentGuilds},
	"STAGE_INSTANCE_UPDATE":             {IntentGuilds},
	"STAGE_INSTANCE_DELETE":             {IntentGuilds},
	"GUILD_ROLE_CREATE":                 {IntentGuilds},
	"GUILD_ROLE_UPDATE":                 {IntentGuilds},
	"GUILD_ROLE_DELETE":                 {IntentGuilds},
	"GUILD_BAN_ADD":                     {IntentGuildModeration},
	"GUILD_BAN_REMOVE":                  {IntentGuildModeration},
	"GUILD_AUDIT_LOG_ENTRY_CREATE":      {IntentGuildModeration},
	"GUILD_EMOJIS_UPDATE":               {IntentGuildExpressions},
	"GUILD_STICKERS_UPDATE":             {IntentGuildExpressions},
	"GUILD_SOUNDBOARD_SOUND_CREATE":     {IntentGuildExpressions},
	"GUILD_SOUNDBOARD_SOUND_UPDATE":     {IntentGuildExpressions},
	"GUILD_SOUNDBOARD_SOUND_DELETE":     {IntentGuildExpressions},
	"GUILD_SOUNDBOARD_SOUNDS_UPDATE":    {IntentGuildExpressions},
	"GUILD_INTEGRATIONS_UPDATE":         {IntentGuildIntegrations},
	"INTEGRATION_CREATE":                {IntentGuildIntegrations},
	"INTEGRATION_UPDATE":                {IntentGuildIntegrations},
	"INTEGRATION_DELETE":                {IntentGuildIntegrations},
	"WEBHOOKS_UPDATE":                   {IntentGuildWebhooks},
	"INVITE_CREATE":                     {IntentGuildInvites},
	"INVITE_DELETE":                     {IntentGuildInvites},
	"VOICE_CHANNEL_EFFECT_SEND":         {IntentGuildVoiceStates},
	"PRESENCE_UPDATE":                   {IntentGuildPresences},
	"GUILD_SCHEDULED_EVENT_CREATE":      {IntentGuildScheduledEvents},
	"GUILD_SCHEDULED_EVENT_UPDATE":      {IntentGuildScheduledEvents},
	"GUILD_SCHEDULED_EVENT_DELETE":      {IntentGuildScheduledEvents},
	"GUILD_SCHEDULED_EVENT_USER_ADD":    {IntentGuildScheduledEvents},
	"GUILD_SCHEDULED_EVENT_USER_REMOVE": {IntentGuildScheduledEvents},
	"AUTO_MODERATION_RULE_CREATE":       {IntentAutoModerationConfiguration},
	"AUTO_MODERATION_RULE_UPDATE":       {IntentAutoModerationConfiguration},
	"AUTO_MODERATION_RULE_DELETE":       {IntentAutoModerationConfiguration},
	"AUTO_MODERATION_ACTION_EXECUTION":  {IntentAutoModerationExecution},
	"MESSAGE_POLL_VOTE_ADD":             {IntentGuildMessagePolls, IntentDirectMessagePolls},
	"MESSAGE_POLL_VOTE_REMOVE":          {IntentGuildMessagePolls, IntentDirectMessagePolls},
	"MESSAGE_REACTION_REMOVE_EMOJI":     {IntentGuildMessageReactions, IntentDirectMessageReactions},

	// READY, RESUMED and INTERACTION_CREATE need no intent at all.
}

// intentNames is used only to make warnings readable.
var intentNames = map[Intent]string{
	IntentGuilds:                      "IntentGuilds",
	IntentGuildMembers:                "IntentGuildMembers",
	IntentGuildMessages:               "IntentGuildMessages",
	IntentGuildVoiceStates:            "IntentGuildVoiceStates",
	IntentGuildModeration:             "IntentGuildModeration",
	IntentGuildExpressions:            "IntentGuildExpressions",
	IntentGuildIntegrations:           "IntentGuildIntegrations",
	IntentGuildWebhooks:               "IntentGuildWebhooks",
	IntentGuildInvites:                "IntentGuildInvites",
	IntentGuildPresences:              "IntentGuildPresences",
	IntentGuildScheduledEvents:        "IntentGuildScheduledEvents",
	IntentAutoModerationConfiguration: "IntentAutoModerationConfiguration",
	IntentAutoModerationExecution:     "IntentAutoModerationExecution",
	IntentGuildMessagePolls:           "IntentGuildMessagePolls",
	IntentDirectMessagePolls:          "IntentDirectMessagePolls",
	IntentGuildMessageReactions:       "IntentGuildMessageReactions",
	IntentGuildMessageTyping:          "IntentGuildMessageTyping",
	IntentDirectMessages:              "IntentDirectMessages",
	IntentDirectMessageReactions:      "IntentDirectMessageReactions",
	IntentDirectMessageTyping:         "IntentDirectMessageTyping",
	IntentMessageContent:              "IntentMessageContent",
}

// intentWarnings reports handlers that will never fire, and message handlers
// that will receive empty text, given the configured intents.
//
// Discord does not complain about either: it simply delivers nothing, or
// delivers messages with the content blanked. Both look identical to a bug in
// your own code, and between them they account for most of the time a new bot
// appears to do nothing at all - so starlings checks before connecting.
func (c *Client) intentWarnings() []string {
	slots := *c.slots.Load()
	if len(slots) == 0 {
		return nil
	}

	var warnings []string
	for event, slot := range slots {
		if len(slot.handlers) == slot.internal {
			continue
		}
		needed, ok := eventIntents[event]
		if !ok {
			continue // no intent gates this event
		}
		if c.intents.HasAny(needed...) {
			continue
		}
		warnings = append(warnings, "handler for "+event+
			" will never fire: enable "+orList(needed))
	}

	// MessageContent is not an event gate - it blanks fields instead, which is
	// even more confusing, so it gets its own check.
	messageSlot, wantsMessages := slots["MESSAGE_CREATE"]
	wantsMessages = wantsMessages && len(messageSlot.handlers) != messageSlot.internal
	if wantsMessages && !c.intents.Has(IntentMessageContent) {
		warnings = append(warnings,
			"MESSAGE_CONTENT is off, so Message.Content will be empty except in "+
				"DMs and messages that mention the bot: enable IntentMessageContent "+
				"(privileged, switch it on in the developer portal too)")
	}

	return warnings
}

// orList renders intents as "IntentGuildMessages or IntentDirectMessages".
func orList(intents []Intent) string {
	names := make([]string, 0, len(intents))
	for _, i := range intents {
		if name, ok := intentNames[i]; ok {
			names = append(names, name)
		}
	}
	switch len(names) {
	case 0:
		return "the required intent"
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
	}
}

// HasAny reports whether at least one of the given intents is enabled.
func (i Intent) HasAny(others ...Intent) bool {
	for _, o := range others {
		if i&o != 0 {
			return true
		}
	}
	return false
}
