# Changelog

## v0.1.2 (unreleased)

This release makes optional features cost nothing for bots that do not use
them.

### Smaller by default

- Voice moved to `github.com/razecrs/starlings/voice`. A bot that does not
  import it no longer links the voice, DAVE, and MLS modules. A minimal bot
  now links three modules instead of sixteen.
- System media detection for Starlog moved to
  `github.com/razecrs/starlings/sysmedia`, so WinRT and D-Bus are linked only
  when it is used.
- Starlog's dashboard renderer is linked only when `WithStarlog` is used.
- The pet pixel art moved to `github.com/razecrs/starlings/starpets`. Import
  it for its side effect to keep the pictured pets; without it, pets use
  terminal frames. The `starlings_small` build tag is no longer needed.
- Pictured pets keep small per-frame images instead of the full decoded
  sheets. Three pets now hold about 1 MB instead of about 18 MB.

### Changed

| v0.1.1 | v0.1.2 |
| --- | --- |
| `bot.ConnectVoice(ctx, guild, channel)` | `voice.Connect(ctx, bot, guild, channel)` |
| `starlings.VoiceConnection` | `voice.Connection` |
| `starlings.OpusProvider`, `OpusPacket` | `voice.OpusProvider`, `voice.OpusPacket` |
| `starlings.NewFFmpegOpusProvider` | `voice.NewFFmpegOpusProvider` |
| `starlings.StarlogSystemMedia()` | `sysmedia.Option()` |
| bundled pet art | `import _ "github.com/razecrs/starlings/starpets"` |

### Fixed

- A timed-out member now keeps only View Channel and Read Message History in
  `BasePermissions` and `Permissions`, as Discord enforces. Channel
  overwrites cannot restore the rest.
- `GuildMemberUpdate` carries the member's full state, including the timeout,
  guild avatar, boost date, pending flag, and member flags. The cache clears
  a value when Discord sends null, so a lifted timeout is no longer kept.
- The bot's own ID is recorded on READY even when users are not cached.

### Gateway

- Gateway commands are paced to Discord's limit of 120 per 60 seconds per
  connection. Heartbeats, identify, and resume keep a reserve, so a burst of
  member requests can no longer disconnect the shard.
- `Disconnect` reports Discord's close code and reason. Close code 4002 now
  says that Discord could not read something the bot sent, and a close that
  repeats after every reconnect is logged once as an error.
- `ShardStatus.Ready` and `Client.Online` follow the live session. They
  become false while a shard is disconnected; `WaitReady` still only waits
  for the first READY.
- `WithMemberChunking` requests the full member list for guilds whose
  GUILD_CREATE was incomplete.

### Interactions

- Slash commands, buttons, selects, and modals are acknowledged
  automatically when the handler has not answered after 2.2 seconds. A fast
  handler still answers in one request. A slow handler's `Reply` or
  `UpdateMessage` edits the deferred response, and an ephemeral reply after a
  public deferral is sent as an ephemeral follow-up. `WithAutoDefer` changes
  the delay, and `NoAutoDefer` turns it off for a command that opens a modal
  after slow work.

### REST

- A route whose limits are not known yet sends one request first, then as
  many as Discord reports, instead of a burst that earns 429s.
- Responses that arrive out of order can no longer reopen a bucket that is
  already used up, and a reset window is refilled only once.
- Routes that Discord reports as one bucket share one local count.
- Interaction callbacks and webhook requests are rate-limited per
  interaction and per webhook, as Discord counts them, instead of in one
  shared bucket.
- Interaction callbacks and follow-ups skip `WithPacing` and the global limit,
  which Discord does not apply to them.
- Bot requests are paced to the global limit of 50 per second before Discord
  answers 429. `WithGlobalRateLimit` changes the rate.
- After Discord answers 401 to the bot token, further requests return
  `ErrTokenRejected` without being sent. `Client.InvalidRequests` reports
  the 401, 403, and 429 count that Cloudflare limits to 10,000 per ten
  minutes.
- `FailFast(ctx)` makes a call return a `*RateLimitError` instead of waiting.
- `RateLimitWait` reports time spent waiting on a known-empty bucket.
- The default HTTP client keeps its connection to Discord for five minutes
  and checks it with HTTP/2 pings.

### Added

- `State.CanModerate` checks the role hierarchy for the member, the target,
  and the bot, and returns a `*ModerationError` whose message can be shown
  to the person who ran the command.
- `Client.MessageHistory` iterates a channel's history with `range`.
- `Client.DownloadAttachment` reads an attachment with a size limit and only
  from Discord's CDN.
- `InteractionCreate.Answered` and `Deferred` report the initial response.
- `Client.ClearTimeout`, `MaxTimeout`, and named error codes for unknown
  bans, webhooks, interactions, and scheduled events.
- `Client.Extension` keeps one value per client for add-on packages and
  closes it with the client.
- `OnOrdered` registers a handler that runs in gateway order before
  application handlers, even with asynchronous events.
- `Client.Starlog`, `StarlogMediaSource`, `StarlogMediaReader`, and
  `RegisterStarPetArt` let other packages extend Starlog.

## v0.1.1

This release is about production behaviour and the parts of the API that felt
longer than they needed to be after building a real bot with v0.1.0.

### Fixed

- Full member requests now send `limit: 0` instead of dropping the field. The
  old payload could make Discord close the gateway with code 4002.
- Autocomplete has its own handler and can no longer run the command handler
  while somebody is still typing.
- Text inputs omit an unset maximum length with `encoding/json/v2`.
- Scheduled-event updates can send a partial PATCH and explicitly clear
  nullable fields.
- The remaining bare user-token gateway path was removed. Social SDK OAuth is
  unchanged.

### Easier to use

- `NewCommandBot` selects the prefix-command intents and starts without an
  unused resource cache.
- Prefix commands can add aliases and `Commands` is stable and sorted.
- Slash commands have chainable permissions, contexts, install types, and a
  separate autocomplete handler.
- `WithCommandSync` can publish commands once after READY.
- Interaction options have direct `String`, `Int`, `Float`, `Bool`, and
  `OptionID` accessors.
- Member avatar URLs, human-readable permission names, missing-permission
  calculation, common Discord error codes, and the generic `Ref` helper fill
  common gaps without removing the underlying fields.

### Guard and StarDB

- Guard measures application handlers and state reads when enabled. It reports
  blocking synchronous handlers, caches that are unused or missing their
  intents, low hit rates, expensive full snapshots, and oversized message
  retention. Reports include bounded runtime memory counters.
- `stardb.Update` adds typed atomic read-modify-write for local files, SQL, and
  Firebase. Unsupported backends return `ErrNotAtomic` instead of pretending a
  read followed by a write is safe.

### License

Starlings is now released under the MIT License instead of AGPL-3.0. You can
use it in closed-source and commercial work; keep the copyright notice.

### Compatibility

`ScheduledEventMetadata` names the type of `ScheduledEvent.EntityMetadata`. It
is an alias of the previous anonymous struct, so v0.1.0 code still compiles.

The member-request payload, modal omission, partial scheduled-event payload,
autocomplete routing, Guard advice, and concurrent file updates all have
regression tests. The full suite passes under the race detector.
