# Discord gateway reference

The OpenAPI spec covers only the REST API, so this file is transcribed from
Discord's gateway documentation and cross-checked against the `ActionTypes`
enum in the spec (see [enums.md](enums.md)), which turned out to list several
events the prose docs omit.

Sources:

- <https://docs.discord.com/developers/events/gateway>
- <https://docs.discord.com/developers/events/gateway-events>
- <https://docs.discord.com/developers/topics/opcodes-and-status-codes>

This is an adapted reference from Discord's developer documentation at commit
`1d752e5e10921e9971c07490a275fa1bf1980a74`, updated to show Starlings'
coverage and implementation notes. Discord's documentation is licensed under
CC BY-SA 4.0; see [the third-party notices](../THIRD_PARTY_NOTICES.md).

The **starlings** column marks what the library models today.

## Opcodes

| code | name | direction | meaning | starlings |
| --- | --- | --- | --- | --- |
| 0 | Dispatch | receive | An event, named by `t` | yes |
| 1 | Heartbeat | both | Keep-alive | yes |
| 2 | Identify | send | Start a new session | yes |
| 3 | Presence Update | send | Change the bot's status | yes |
| 4 | Voice State Update | send | Join, leave or move voice channels | yes |
| 6 | Resume | send | Resume a dropped session | yes |
| 7 | Reconnect | receive | Reconnect and resume immediately | yes |
| 8 | Request Guild Members | send | Ask for a large guild's member list | yes |
| 9 | Invalid Session | receive | Session is dead | yes |
| 10 | Hello | receive | First frame; carries `heartbeat_interval` | yes |
| 11 | Heartbeat ACK | receive | Acknowledges a heartbeat | yes |
| 31 | Request Soundboard Sounds | send | Soundboard sounds for a set of guilds | yes |
| 43 | Request Channel Info | send | Ephemeral channel data for a guild | yes |

## Close codes

`reconnect: no` means the bot is misconfigured and retrying will fail
identically forever - starlings surfaces these as `*FatalError` and stops.

| code | meaning | reconnect | starlings |
| --- | --- | --- | --- |
| 4000 | Unknown error | yes | yes |
| 4001 | Unknown opcode | yes | yes |
| 4002 | Decode error | yes | yes |
| 4003 | Not authenticated | yes | yes |
| 4004 | Authentication failed - bad token | **no** | yes |
| 4005 | Already authenticated | yes | yes |
| 4007 | Invalid seq during resume | yes | yes |
| 4008 | Rate limited | yes | yes |
| 4009 | Session timed out | yes | yes |
| 4010 | Invalid shard | **no** | yes |
| 4011 | Sharding required | **no** | yes |
| 4012 | Invalid API version | **no** | yes |
| 4013 | Invalid intents | **no** | yes |
| 4014 | Disallowed intents - not enabled in the portal | **no** | yes |

## Intents

A bitmask sent at identify. An event you have no intent for is simply never
delivered. **Privileged** intents need switching on in the developer portal,
and approval once the bot is in 100+ guilds.

| bit | intent | privileged | gates |
| --- | --- | --- | --- |
| 1<<0 | GUILDS | | `GUILD_CREATE` `GUILD_UPDATE` `GUILD_DELETE` `GUILD_ROLE_CREATE` `GUILD_ROLE_UPDATE` `GUILD_ROLE_DELETE` `CHANNEL_CREATE` `CHANNEL_UPDATE` `CHANNEL_DELETE` `CHANNEL_PINS_UPDATE` `THREAD_CREATE` `THREAD_UPDATE` `THREAD_DELETE` `THREAD_LIST_SYNC` `THREAD_MEMBER_UPDATE` `THREAD_MEMBERS_UPDATE` `STAGE_INSTANCE_CREATE` `STAGE_INSTANCE_UPDATE` `STAGE_INSTANCE_DELETE` `VOICE_CHANNEL_STATUS_UPDATE` `VOICE_CHANNEL_START_TIME_UPDATE` |
| 1<<1 | GUILD_MEMBERS | **yes** | `GUILD_MEMBER_ADD` `GUILD_MEMBER_UPDATE` `GUILD_MEMBER_REMOVE` `THREAD_MEMBERS_UPDATE` |
| 1<<2 | GUILD_MODERATION | | `GUILD_AUDIT_LOG_ENTRY_CREATE` `GUILD_BAN_ADD` `GUILD_BAN_REMOVE` |
| 1<<3 | GUILD_EXPRESSIONS | | `GUILD_EMOJIS_UPDATE` `GUILD_STICKERS_UPDATE` `GUILD_SOUNDBOARD_SOUND_CREATE` `GUILD_SOUNDBOARD_SOUND_UPDATE` `GUILD_SOUNDBOARD_SOUND_DELETE` `GUILD_SOUNDBOARD_SOUNDS_UPDATE` |
| 1<<4 | GUILD_INTEGRATIONS | | `GUILD_INTEGRATIONS_UPDATE` `INTEGRATION_CREATE` `INTEGRATION_UPDATE` `INTEGRATION_DELETE` |
| 1<<5 | GUILD_WEBHOOKS | | `WEBHOOKS_UPDATE` |
| 1<<6 | GUILD_INVITES | | `INVITE_CREATE` `INVITE_DELETE` |
| 1<<7 | GUILD_VOICE_STATES | | `VOICE_CHANNEL_EFFECT_SEND` `VOICE_STATE_UPDATE` |
| 1<<8 | GUILD_PRESENCES | **yes** | `PRESENCE_UPDATE` |
| 1<<9 | GUILD_MESSAGES | | `MESSAGE_CREATE` `MESSAGE_UPDATE` `MESSAGE_DELETE` `MESSAGE_DELETE_BULK` |
| 1<<10 | GUILD_MESSAGE_REACTIONS | | `MESSAGE_REACTION_ADD` `MESSAGE_REACTION_REMOVE` `MESSAGE_REACTION_REMOVE_ALL` `MESSAGE_REACTION_REMOVE_EMOJI` |
| 1<<11 | GUILD_MESSAGE_TYPING | | `TYPING_START` |
| 1<<12 | DIRECT_MESSAGES | | `MESSAGE_CREATE` `MESSAGE_UPDATE` `MESSAGE_DELETE` `CHANNEL_PINS_UPDATE` |
| 1<<13 | DIRECT_MESSAGE_REACTIONS | | the four `MESSAGE_REACTION_*` events, in DMs |
| 1<<14 | DIRECT_MESSAGE_TYPING | | `TYPING_START` in DMs |
| 1<<15 | MESSAGE_CONTENT | **yes** | not an event gate - without it, `content`, `embeds`, `attachments` and `components` arrive empty on messages the bot is not mentioned in |
| 1<<16 | GUILD_SCHEDULED_EVENTS | | `GUILD_SCHEDULED_EVENT_CREATE` `GUILD_SCHEDULED_EVENT_UPDATE` `GUILD_SCHEDULED_EVENT_DELETE` `GUILD_SCHEDULED_EVENT_USER_ADD` `GUILD_SCHEDULED_EVENT_USER_REMOVE` |
| 1<<20 | AUTO_MODERATION_CONFIGURATION | | `AUTO_MODERATION_RULE_CREATE` `AUTO_MODERATION_RULE_UPDATE` `AUTO_MODERATION_RULE_DELETE` |
| 1<<21 | AUTO_MODERATION_EXECUTION | | `AUTO_MODERATION_ACTION_EXECUTION` |
| 1<<24 | GUILD_MESSAGE_POLLS | | `MESSAGE_POLL_VOTE_ADD` `MESSAGE_POLL_VOTE_REMOVE` |
| 1<<25 | DIRECT_MESSAGE_POLLS | | the poll vote events, in DMs |

Bits 17, 18, 19, 22 and 23 are unassigned. Starlings' `Intent` constants match this
table exactly, including the gaps.

## Dispatch events

Every documented `op: 0` event name. **Starlings models 79 of 88.** Two more
have raw delivery, and the remaining seven are Social SDK lobby,
`GAME_DIRECT_MESSAGE_*`, and quest events that are outside the ordinary bot
gateway surface.

### Lifecycle

| event | starlings |
| --- | --- |
| `READY` | yes |
| `RESUMED` | yes |
| `RATE_LIMITED` | yes |

### Application and interactions

| event | starlings |
| --- | --- |
| `INTERACTION_CREATE` | yes |
| `APPLICATION_COMMAND_PERMISSIONS_UPDATE` | yes |
| `APPLICATION_AUTHORIZED` | raw (`OnRaw`) |
| `APPLICATION_DEAUTHORIZED` | raw (`OnRaw`) |

### Auto moderation

| event | starlings |
| --- | --- |
| `AUTO_MODERATION_RULE_CREATE` | yes |
| `AUTO_MODERATION_RULE_UPDATE` | yes |
| `AUTO_MODERATION_RULE_DELETE` | yes |
| `AUTO_MODERATION_ACTION_EXECUTION` | yes |

### Channels and threads

| event | starlings |
| --- | --- |
| `CHANNEL_CREATE` | yes |
| `CHANNEL_UPDATE` | yes |
| `CHANNEL_DELETE` | yes |
| `CHANNEL_INFO` | yes |
| `CHANNEL_PINS_UPDATE` | yes |
| `THREAD_CREATE` | yes |
| `THREAD_UPDATE` | yes |
| `THREAD_DELETE` | yes |
| `THREAD_LIST_SYNC` | yes |
| `THREAD_MEMBER_UPDATE` | yes |
| `THREAD_MEMBERS_UPDATE` | yes |

### Guilds

| event | starlings |
| --- | --- |
| `GUILD_CREATE` | yes |
| `GUILD_UPDATE` | yes |
| `GUILD_DELETE` | yes |
| `GUILD_AUDIT_LOG_ENTRY_CREATE` | yes |
| `GUILD_BAN_ADD` | yes |
| `GUILD_BAN_REMOVE` | yes |
| `GUILD_EMOJIS_UPDATE` | yes |
| `GUILD_STICKERS_UPDATE` | yes |
| `GUILD_INTEGRATIONS_UPDATE` | yes |
| `GUILD_MEMBER_ADD` | yes |
| `GUILD_MEMBER_REMOVE` | yes |
| `GUILD_MEMBER_UPDATE` | yes |
| `GUILD_MEMBERS_CHUNK` | yes |
| `GUILD_ROLE_CREATE` | yes |
| `GUILD_ROLE_UPDATE` | yes |
| `GUILD_ROLE_DELETE` | yes |

### Scheduled events

| event | starlings |
| --- | --- |
| `GUILD_SCHEDULED_EVENT_CREATE` | yes |
| `GUILD_SCHEDULED_EVENT_UPDATE` | yes |
| `GUILD_SCHEDULED_EVENT_DELETE` | yes |
| `GUILD_SCHEDULED_EVENT_USER_ADD` | yes |
| `GUILD_SCHEDULED_EVENT_USER_REMOVE` | yes |

### Soundboard

| event | starlings |
| --- | --- |
| `GUILD_SOUNDBOARD_SOUND_CREATE` | yes |
| `GUILD_SOUNDBOARD_SOUND_UPDATE` | yes |
| `GUILD_SOUNDBOARD_SOUND_DELETE` | yes |
| `GUILD_SOUNDBOARD_SOUNDS_UPDATE` | yes |
| `SOUNDBOARD_SOUNDS` | yes |

### Integrations, invites, webhooks

| event | starlings |
| --- | --- |
| `INTEGRATION_CREATE` | yes |
| `INTEGRATION_UPDATE` | yes |
| `INTEGRATION_DELETE` | yes |
| `INVITE_CREATE` | yes |
| `INVITE_DELETE` | yes |
| `WEBHOOKS_UPDATE` | yes |

### Messages

| event | starlings |
| --- | --- |
| `MESSAGE_CREATE` | yes |
| `MESSAGE_UPDATE` | yes |
| `MESSAGE_DELETE` | yes |
| `MESSAGE_DELETE_BULK` | yes |
| `MESSAGE_REACTION_ADD` | yes |
| `MESSAGE_REACTION_REMOVE` | yes |
| `MESSAGE_REACTION_REMOVE_ALL` | yes |
| `MESSAGE_REACTION_REMOVE_EMOJI` | yes |
| `MESSAGE_POLL_VOTE_ADD` | yes |
| `MESSAGE_POLL_VOTE_REMOVE` | yes |

### Monetisation

| event | starlings |
| --- | --- |
| `ENTITLEMENT_CREATE` | yes |
| `ENTITLEMENT_UPDATE` | yes |
| `ENTITLEMENT_DELETE` | yes |
| `SUBSCRIPTION_CREATE` | yes |
| `SUBSCRIPTION_UPDATE` | yes |
| `SUBSCRIPTION_DELETE` | yes |

### Presence, stage, users

| event | starlings |
| --- | --- |
| `PRESENCE_UPDATE` | yes |
| `TYPING_START` | yes |
| `USER_UPDATE` | yes |
| `STAGE_INSTANCE_CREATE` | yes |
| `STAGE_INSTANCE_UPDATE` | yes |
| `STAGE_INSTANCE_DELETE` | yes |

### Voice

| event | starlings |
| --- | --- |
| `VOICE_STATE_UPDATE` | yes |
| `VOICE_SERVER_UPDATE` | yes |
| `VOICE_CHANNEL_EFFECT_SEND` | yes |
| `VOICE_CHANNEL_STATUS_UPDATE` | yes |
| `VOICE_CHANNEL_START_TIME_UPDATE` | yes |

### Social SDK and lobbies

These appear in the spec's `ActionTypes` enum but not in the prose event docs.
They belong to the Social SDK surface and are irrelevant to ordinary bots.

`LOBBY_MESSAGE_CREATE` `LOBBY_MESSAGE_UPDATE` `LOBBY_MESSAGE_DELETE`
`GAME_DIRECT_MESSAGE_CREATE` `GAME_DIRECT_MESSAGE_UPDATE`
`GAME_DIRECT_MESSAGE_DELETE` `QUEST_USER_ENROLLMENT`

## Voice gateway

A separate websocket with its own opcodes, reached after pairing
`VOICE_STATE_UPDATE` with `VOICE_SERVER_UPDATE`. `voice.Connect` handles this
gateway, UDP discovery, transport encryption, DAVE setup, heartbeats,
reconnects, speaking state, and session descriptions. Applications normally
use `PlayFile`, `Play`, or `Receive` rather than sending these opcodes.

| code | name |
| --- | --- |
| 0 | Identify |
| 1 | Select Protocol |
| 2 | Ready |
| 3 | Heartbeat |
| 4 | Session Description |
| 5 | Speaking |
| 6 | Heartbeat ACK |
| 7 | Resume |
| 8 | Hello |
| 9 | Resumed |
| 11 | Clients Connect |
| 13 | Client Disconnect |
| 21–31 | DAVE end-to-end encryption protocol |

Voice close codes: 4001 unknown opcode, 4002 decode failure, 4003 not
authenticated, 4004 auth failed, 4005 already authenticated, 4006 session no
longer valid, 4009 session timeout, 4011 server not found, 4012 unknown
protocol, 4014 disconnected, 4015 server crashed, 4016 unknown encryption mode,
4017 E2EE/DAVE required, 4020 bad request, 4021 rate limited, 4022 call
terminated.

Reconnecting is inadvisable after 4014, 4021 and 4022; 4015 should resume.

## HTTP status codes

| code | meaning |
| --- | --- |
| 200 | OK |
| 201 | Created |
| 204 | No content |
| 304 | Not modified |
| 400 | Bad request - malformed payload |
| 401 | Unauthorized - missing or invalid token |
| 403 | Forbidden - token valid, permission missing |
| 404 | Not found |
| 405 | Method not allowed |
| 429 | Rate limited |
| 502 | No gateway available to process the request |
| 5xx | Server error |

starlings maps these onto `*APIError` with `HTTPStatus`, `IsNotFound`,
`IsUnauthorized` and `IsForbidden` helpers, retries 429 and 5xx, and returns
4xx directly.
