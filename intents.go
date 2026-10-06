package starlings

// Intent is a bitmask telling the gateway which families of events a bot wants
// to receive. Discord requires you to ask for what you need: an event you have
// no intent for is simply never sent.
//
// Combine them with |, and pass the result to Config.Intents:
//
//	IntentGuilds | IntentGuildMessages | IntentMessageContent
type Intent uint64

const (
	IntentGuilds Intent = 1 << iota
	IntentGuildMembers
	IntentGuildModeration
	IntentGuildExpressions
	IntentGuildIntegrations
	IntentGuildWebhooks
	IntentGuildInvites
	IntentGuildVoiceStates
	IntentGuildPresences
	IntentGuildMessages
	IntentGuildMessageReactions
	IntentGuildMessageTyping
	IntentDirectMessages
	IntentDirectMessageReactions
	IntentDirectMessageTyping
	IntentMessageContent
	IntentGuildScheduledEvents
	_
	_
	_
	IntentAutoModerationConfiguration
	IntentAutoModerationExecution
	_
	_
	IntentGuildMessagePolls
	IntentDirectMessagePolls
)

// IntentsNone receives only events that need no intent at all, such as
// interactions from slash commands.
const IntentsNone Intent = 0

// IntentsPrivileged is the set Discord gates behind extra approval. A bot in
// more than 100 guilds must be verified and have these switched on in the
// developer portal; below that, they still have to be enabled there manually.
const IntentsPrivileged = IntentGuildMembers | IntentGuildPresences | IntentMessageContent

// IntentsSelfbot is every intent a user (selfbot) token receives. A bot app
// must have the privileged ones switched on in the developer portal; a user
// token has no portal, so Selfbot asks for all of them and every event
// family just works.
const IntentsSelfbot = IntentsAllUnprivileged | IntentGuildMembers | IntentGuildPresences | IntentMessageContent

// IntentsAllUnprivileged is every intent that works without special approval.
const IntentsAllUnprivileged = IntentGuilds | IntentGuildModeration | IntentGuildExpressions |
	IntentGuildIntegrations | IntentGuildWebhooks | IntentGuildInvites | IntentGuildVoiceStates |
	IntentGuildMessages | IntentGuildMessageReactions | IntentGuildMessageTyping |
	IntentDirectMessages | IntentDirectMessageReactions | IntentDirectMessageTyping |
	IntentGuildScheduledEvents | IntentAutoModerationConfiguration |
	IntentAutoModerationExecution | IntentGuildMessagePolls | IntentDirectMessagePolls

// Has reports whether every intent in other is present in i.
func (i Intent) Has(other Intent) bool { return i&other == other }
