package starlings

import (
	"errors"
	"sync"
)

// StateMode controls who applies gateway events to State.
type StateMode uint8

const (
	// StateAutomatic keeps State synchronized before user handlers run.
	StateAutomatic StateMode = iota
	// StateManual leaves synchronization to State.Apply calls in user code.
	StateManual
)

// StateConfig controls which gateway resources Starlings retains.
//
// Start with DefaultStateConfig and turn off resources a bot does not need.
// A zero MaxMessagesPerChannel disables the message cache.
type StateConfig struct {
	Mode StateMode

	Guilds        bool
	Channels      bool
	Members       bool
	Users         bool
	Roles         bool
	Emojis        bool
	Stickers      bool
	ThreadMembers bool
	VoiceStates   bool
	Presences     bool

	MaxMessagesPerChannel int
}

// DefaultStateConfig returns Starlings' complete zero-configuration cache.
func DefaultStateConfig() StateConfig {
	return StateConfig{
		Mode:                  StateAutomatic,
		Guilds:                true,
		Channels:              true,
		Members:               true,
		Users:                 true,
		Roles:                 true,
		Emojis:                true,
		Stickers:              true,
		ThreadMembers:         true,
		VoiceStates:           true,
		Presences:             true,
		MaxMessagesPerChannel: 100,
	}
}

// MinimalStateConfig disables resource retention while leaving automatic
// mode selected. Use it for stateless interaction bots, workers, and prefix
// bots that only react to the event currently being handled.
func MinimalStateConfig() StateConfig { return StateConfig{Mode: StateAutomatic} }

// WithStateCache replaces the default cache configuration.
func WithStateCache(config StateConfig) Option {
	return func(c *Client) { c.State = newStateWithConfig(config) }
}

// WithStateMode switches between automatic and manual event application while
// preserving the rest of the cache configuration.
func WithStateMode(mode StateMode) Option {
	return func(c *Client) {
		config := c.State.Config()
		config.Mode = mode
		c.State = newStateWithConfig(config)
	}
}

// State is Starlings' concurrency-safe in-memory view of Discord.
// Returned objects are snapshots: callers may mutate them safely.
type State struct {
	mu     sync.RWMutex
	config StateConfig
	guard  *Guard
	selfID Snowflake

	guilds        map[Snowflake]Guild
	loadedGuilds  map[Snowflake]struct{}
	channels      map[Snowflake]Channel
	members       map[Snowflake]map[Snowflake]Member
	users         map[Snowflake]User
	roles         map[Snowflake]map[Snowflake]Role
	emojis        map[Snowflake]map[Snowflake]GuildEmoji
	stickers      map[Snowflake]map[Snowflake]Sticker
	threadMembers map[Snowflake]map[Snowflake]ThreadMember
	threadGuild   map[Snowflake]Snowflake
	threadParent  map[Snowflake]Snowflake
	voiceStates   map[Snowflake]map[Snowflake]VoiceState
	presences     map[Snowflake]map[Snowflake]PresenceUpdate
	messages      map[Snowflake]*channelMessages
	messageGuild  map[Snowflake]Snowflake
}

type channelMessages struct {
	order []Snowflake
	items map[Snowflake]Message
}

func newState() *State { return newStateWithConfig(DefaultStateConfig()) }

func newStateWithConfig(config StateConfig) *State {
	if config.MaxMessagesPerChannel < 0 {
		config.MaxMessagesPerChannel = 0
	}
	return &State{
		config:        config,
		guilds:        make(map[Snowflake]Guild),
		loadedGuilds:  make(map[Snowflake]struct{}),
		channels:      make(map[Snowflake]Channel),
		members:       make(map[Snowflake]map[Snowflake]Member),
		users:         make(map[Snowflake]User),
		roles:         make(map[Snowflake]map[Snowflake]Role),
		emojis:        make(map[Snowflake]map[Snowflake]GuildEmoji),
		stickers:      make(map[Snowflake]map[Snowflake]Sticker),
		threadMembers: make(map[Snowflake]map[Snowflake]ThreadMember),
		threadGuild:   make(map[Snowflake]Snowflake),
		threadParent:  make(map[Snowflake]Snowflake),
		voiceStates:   make(map[Snowflake]map[Snowflake]VoiceState),
		presences:     make(map[Snowflake]map[Snowflake]PresenceUpdate),
		messages:      make(map[Snowflake]*channelMessages),
		messageGuild:  make(map[Snowflake]Snowflake),
	}
}

// Config returns a copy of the active cache configuration.
func (s *State) Config() StateConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// Mode reports whether events are applied automatically or manually.
func (s *State) Mode() StateMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.Mode
}

// The small locked helpers keep internal tests and package-level integrations
// on the same mutation path as Apply.
func (s *State) putGuild(guild Guild) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putGuildLocked(guild)
}

func (s *State) markGuildUnavailable(id Snowflake) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if guild, ok := s.guilds[id]; ok {
		guild.Unavailable = true
		s.guilds[id] = guild
	}
}

func (s *State) removeGuild(id Snowflake) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeGuildLocked(id)
}

// ErrUnsupportedStateEvent is returned when Apply receives an event with no
// state mutation. Generic dispatchers may safely ignore this sentinel.
var ErrUnsupportedStateEvent = errors.New("starlings: event does not change cached state")

func (c *Client) installStateHandlers() {
	if c.State == nil {
		return
	}
	apply := func(event Event) { _ = c.State.Apply(event) }
	if c.State.Mode() == StateManual {
		internalOn(c, func(e *UserUpdate) { c.self.Store(&e.User) })
		return
	}
	config := c.State.Config()
	if config.Guilds || config.Users {
		internalOn(c, func(e *Ready) { apply(e) })
	}
	guildResources := config.Guilds || config.Channels || config.Members || config.Users ||
		config.Roles || config.Emojis || config.Stickers || config.ThreadMembers ||
		config.VoiceStates || config.Presences
	if guildResources {
		internalOn(c, func(e *GuildCreate) { apply(e) })
		internalOn(c, func(e *GuildDelete) { apply(e) })
	}
	if config.Guilds {
		internalOn(c, func(e *GuildUpdate) { apply(e) })
	}
	if config.Channels {
		internalOn(c, func(e *ChannelCreate) { apply(e) })
		internalOn(c, func(e *ChannelUpdate) { apply(e) })
		internalOn(c, func(e *ChannelDelete) { apply(e) })
		internalOn(c, func(e *ChannelPinsUpdate) { apply(e) })
		internalOn(c, func(e *ChannelInfo) { apply(e) })
		internalOn(c, func(e *VoiceChannelStatusUpdate) { apply(e) })
		internalOn(c, func(e *VoiceChannelStartTimeUpdate) { apply(e) })
	}
	if config.Channels || config.ThreadMembers {
		internalOn(c, func(e *ThreadCreate) { apply(e) })
		internalOn(c, func(e *ThreadUpdate) { apply(e) })
		internalOn(c, func(e *ThreadDelete) { apply(e) })
		internalOn(c, func(e *ThreadListSync) { apply(e) })
	}
	if config.ThreadMembers {
		internalOn(c, func(e *ThreadMemberUpdate) { apply(e) })
		internalOn(c, func(e *ThreadMembersUpdate) { apply(e) })
	}
	if config.Members || config.Users {
		internalOn(c, func(e *GuildMemberAdd) { apply(e) })
		internalOn(c, func(e *GuildMemberUpdate) { apply(e) })
		internalOn(c, func(e *GuildMemberRemove) { apply(e) })
		internalOn(c, func(e *GuildMembersChunk) { apply(e) })
	}
	if config.Roles {
		internalOn(c, func(e *GuildRoleCreate) { apply(e) })
		internalOn(c, func(e *GuildRoleUpdate) { apply(e) })
		internalOn(c, func(e *GuildRoleDelete) { apply(e) })
	}
	if config.Emojis {
		internalOn(c, func(e *GuildEmojisUpdate) { apply(e) })
	}
	if config.Stickers {
		internalOn(c, func(e *GuildStickersUpdate) { apply(e) })
	}
	if config.VoiceStates || config.Members || config.Users {
		internalOn(c, func(e *VoiceStateUpdate) { apply(e) })
	}
	if config.Presences || config.Users {
		internalOn(c, func(e *PresenceUpdate) { apply(e) })
		internalOn(c, func(e *PresencesReplace) { apply(e) })
	}
	if config.MaxMessagesPerChannel > 0 || config.Members || config.Users {
		internalOn(c, func(e *MessageCreate) { apply(e) })
	}
	if config.MaxMessagesPerChannel > 0 {
		internalOn(c, func(e *MessageUpdate) { apply(e) })
		internalOn(c, func(e *MessageDelete) { apply(e) })
		internalOn(c, func(e *MessageDeleteBulk) { apply(e) })
		internalOn(c, func(e *MessageReactionAdd) { apply(e) })
		internalOn(c, func(e *MessageReactionRemove) { apply(e) })
		internalOn(c, func(e *MessageReactionRemoveAll) { apply(e) })
		internalOn(c, func(e *MessageReactionRemoveEmoji) { apply(e) })
	}
	internalOn(c, func(e *UserUpdate) {
		apply(e)
		c.self.Store(&e.User)
	})
}
