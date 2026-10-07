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

// IntentsAll is every intent there is, privileged ones included. Convenient
// while developing, when you would rather see every event than work out which
// intent carries the one you want.
//
// The privileged three still have to be switched on in the developer portal,
// and Discord closes the connection with an Invalid Intents code if they are
// not, so narrow this down to what you actually handle before shipping.
const IntentsAll = IntentsAllUnprivileged | IntentsPrivileged

// IntentsAllUnprivileged is every intent that works without special approval.
const IntentsAllUnprivileged = IntentGuilds | IntentGuildModeration | IntentGuildExpressions |
	IntentGuildIntegrations | IntentGuildWebhooks | IntentGuildInvites | IntentGuildVoiceStates |
	IntentGuildMessages | IntentGuildMessageReactions | IntentGuildMessageTyping |
	IntentDirectMessages | IntentDirectMessageReactions | IntentDirectMessageTyping |
	IntentGuildScheduledEvents | IntentAutoModerationConfiguration |
	IntentAutoModerationExecution | IntentGuildMessagePolls | IntentDirectMessagePolls

// Has reports whether every intent in other is present in i.
func (i Intent) Has(other Intent) bool { return i&other == other }
