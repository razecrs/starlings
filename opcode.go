package starlings

// Opcode identifies what a gateway frame is for. The payload's meaning depends
// entirely on it.
type Opcode int

const (
	OpDispatch                Opcode = 0  // server -> client: an event, named by "t"
	OpHeartbeat               Opcode = 1  // both ways: keep-alive
	OpIdentify                Opcode = 2  // client -> server: open a new session
	OpPresenceUpdate          Opcode = 3  // client -> server: change status
	OpVoiceStateUpdate        Opcode = 4  // client -> server: join or leave a voice channel
	OpResume                  Opcode = 6  // client -> server: resume a dropped session
	OpReconnect               Opcode = 7  // server -> client: reconnect and resume
	OpRequestGuildMembers     Opcode = 8  // client -> server: ask for a guild's member list
	OpInvalidSession          Opcode = 9  // server -> client: session is dead
	OpHello                   Opcode = 10 // server -> client: first frame, carries the heartbeat interval
	OpHeartbeatACK            Opcode = 11 // server -> client: heartbeat received
	OpRequestSoundboardSounds Opcode = 31 // client -> server: fetch soundboard sounds
	OpRequestChannelInfo      Opcode = 43 // client -> server: fetch ephemeral channel data
)

// CloseCode is a websocket close code from Discord's gateway.
type CloseCode int

const (
	CloseUnknownError CloseCode = 4000 + iota
	CloseUnknownOpcode
	CloseDecodeError
	CloseNotAuthenticated
	CloseAuthenticationFailed
	CloseAlreadyAuthenticated
	_
	CloseInvalidSeq
	CloseRateLimited
	CloseSessionTimedOut
	CloseInvalidShard
	CloseShardingRequired
	CloseInvalidAPIVersion
	CloseInvalidIntents
	CloseDisallowedIntents
)

// Resumable reports whether reconnecting with the same session could work.
// When it does not, the client has to identify afresh.
func (c CloseCode) Resumable() bool {
	switch c {
	case CloseAuthenticationFailed, CloseInvalidShard, CloseShardingRequired,
		CloseInvalidAPIVersion, CloseInvalidIntents, CloseDisallowedIntents:
		return false
	}
	return true
}

// Fatal reports whether reconnecting at all is pointless - the bot is
// misconfigured and will fail the same way every time.
func (c CloseCode) Fatal() bool {
	switch c {
	case CloseAuthenticationFailed, CloseInvalidShard, CloseShardingRequired,
		CloseInvalidAPIVersion, CloseInvalidIntents, CloseDisallowedIntents:
		return true
	}
	return false
}

func (c CloseCode) String() string {
	switch c {
	case CloseUnknownError:
		return "unknown error"
	case CloseUnknownOpcode:
		return "unknown opcode"
	case CloseDecodeError:
		return "decode error: Discord could not read a payload this bot sent"
	case CloseNotAuthenticated:
		return "not authenticated"
	case CloseAuthenticationFailed:
		return "authentication failed: check the bot token"
	case CloseAlreadyAuthenticated:
		return "already authenticated"
	case CloseInvalidSeq:
		return "invalid sequence"
	case CloseRateLimited:
		return "rate limited: more than 120 gateway commands in 60 seconds"
	case CloseSessionTimedOut:
		return "session timed out"
	case CloseInvalidShard:
		return "invalid shard"
	case CloseShardingRequired:
		return "sharding required: this bot is in too many guilds for one connection"
	case CloseInvalidAPIVersion:
		return "invalid API version"
	case CloseInvalidIntents:
		return "invalid intents"
	case CloseDisallowedIntents:
		return "disallowed intents: enable them in the developer portal"
	default:
		return "close code " + itoa(int(c))
	}
}
