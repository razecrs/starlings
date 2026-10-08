# Starlings documentation

Start with [getting started](getting-started.md), then use the guide for the part of the bot you are building.

## Build a bot

- [Getting started](getting-started.md): token, intents, commands, events, shutdown, and the first production checks.
- [Interactions](interactions.md): slash commands, typed arguments, handler results, errors, syncing, embeds, buttons, modals, pagers, autocomplete, and HTTP delivery.
- [Messages and REST](rest-and-messages.md): sending, values that act on themselves, history, purging, uploads, downloads, mentions, errors, rate limits, audit reasons, and raw routes.
- [Voice](voice.md): joining, FFmpeg playback, custom Opus, receiving packets, soundboard, and failure handling.
- [Runtime and gateway](runtime.md): reconnects, readiness, what happens automatically and how to turn it off, member lists, sharding, dispatch ordering, raw events, and extension hooks.

## Optional systems

- [State and Starlings Guard](state.md): cache contents, snapshots, permissions, timeouts, moderation checks, manual mode, and implementation comparisons.
- [Starlog](starlog.md): dashboard, streaming mode, pets, media, sinks, subprocess output, and deployment behaviour.
- [StarDB](stardb.md): JSON, CSV, SQL, Firebase, Supabase, encryption, limits, and logging.
- [Social SDK](social-sdk.md): bot/user authorization boundaries, lobbies, provisional accounts, and profile widgets.

## Reference and migration

- [Moving from DiscordGo](migrating-from-discordgo.md): equivalent setup, handlers, state, REST, and voice.
- [Gateway event reference](gateway-events.md)
- [REST route coverage](coverage.md) and [complete REST surface](rest-surface.md)
- [Object schemas](schemas.md) and [enumerations](enums.md)
- [Protocol research](research.md)
- [Arena methodology](../benchmarks/arena/METHODOLOGY.md) and [results](../benchmarks/arena/results.md)

The generated references cover breadth. The task guides explain lifecycle, safety, and the choices that are easy to get wrong. Exported Go declarations retain their detailed package documentation for editors and [pkg.go.dev](https://pkg.go.dev/github.com/razecrs/starlings).
