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

### Added

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
