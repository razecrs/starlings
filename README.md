# Starlings

<img src="assets/starlings-logo.png" alt="Starlings logo" width="180">

Starlings is a Discord library for Go. It keeps the common bot code short without hiding the gateway, state, voice, or REST API when you need control.

```go
bot := starlings.NewCommandBot(os.Getenv("DISCORD_TOKEN"))

bot.Command("ping", func(m *starlings.MessageCreate, _ []string) {
	m.Reply("pong")
})

log.Fatal(bot.Run())
```

`Run` handles reconnects, session resumes, gateway compression, and Discord's recommended shard count. A small bot can stay small; a large one does not need a different client.

## Start here

Starlings requires Go 1.27 or newer.

```sh
go get github.com/razecrs/starlings@latest
```

Set `DISCORD_TOKEN`, enable the Message Content intent in the Discord developer portal, then run the first example:

```sh
go run ./examples/firstbot
```

The token belongs in the environment, never in source. Copy [`.env.example`](.env.example) for the larger examples that load several settings.

## The short path

Commands do not need event plumbing:

```go
bot.Command("say", func(m *starlings.MessageCreate, args []string) {
	m.ReplyComplex(starlings.SendData{
		Content:         strings.Join(args, " "),
		AllowedMentions: starlings.NoMentions(),
	})
})
```

`NewCommandBot` selects the three intents prefix commands need and skips the
resource cache they usually do not. It is only a preset: options can replace
both choices, or use `New` for complete state. Message Content must still be
enabled in Discord's developer portal.

Typed handlers infer the event from the argument:

```go
bot.On(func(r *starlings.Ready) { log.Printf("online as %s", r.User.Tag()) })
bot.On(func(m *starlings.MessageCreate) { /* ... */ })
```

For compile-time handler checks, use `starlings.On(bot, handler)`. `Listen` returns an unsubscribe function and `Once` removes itself after the first event.

Interactions use the same client:

```go
bot.Slash("hello", "Say hello", func(i *starlings.InteractionCreate) {
	if err := i.Reply("hello"); err != nil {
		log.Print(err)
	}
})

bot.On(func(*starlings.Ready) {
	if err := bot.SyncCommands(context.Background(), 0); err != nil {
		log.Print(err)
	}
})
```

They can arrive through the gateway or a verified HTTP endpoint. `InteractionHandler` is a normal `net/http` handler; `VerifyInteraction` is available when the application owns request routing.

For a small bot, `WithCommandSync(guildID)` publishes registered commands once
after READY. Leave it out when deployment code should control command changes.

## What is covered

- Gateway identify, resume, heartbeat recovery, zlib-stream compression, reconnect backoff, and automatic or manual sharding.
- Typed dispatch for 79 documented gateway events, plus raw event and raw frame escape hatches.
- A configurable concurrent state cache for guilds, channels, threads, members, users, roles, emoji, stickers, thread members, voice states, presences, and bounded message history.
- Effective permissions, user colours, and previous-object snapshots on update and delete events.
- REST rate-limit buckets, global limits, 429 and transient 5xx retries, file uploads, and audit-log reasons.
- Slash commands, components, modals, autocomplete, followups, polls, Components V2, and verified HTTP interactions.
- Voice gateway v8, UDP discovery, current AEAD transport modes, DAVE E2EE, Opus send/receive, and one-call FFmpeg playback.
- Soundboard, application commands, AutoMod, webhooks, guild administration, monetisation, profile widgets, and Social SDK server routes.

The generated REST audit currently finds a Starlings route for all 246 operations in Discord's OpenAPI specification. That is source-surface coverage, not a claim that every route has been exercised against a live application. Undocumented and experimental routes are reported separately instead of being counted. See [the coverage report](docs/coverage.md) and regenerate it with `go run ./internal/apidoc`.

## State without a black box

The default cache is ready without setup:

```go
channel, ok := bot.State.Channel(channelID)
member, ok := bot.State.Member(guildID, userID)
permissions, err := bot.State.ChannelPermissions(channelID, userID)
colour, err := bot.State.UserColor(guildID, userID)
```

Lookups return safe snapshots rather than writable internal maps. Cache categories and message limits are individually configurable. Manual mode runs the same mutation code explicitly when an application needs to own timing:

```go
bot := starlings.New(token,
	starlings.WithStateMode(starlings.StateManual),
	starlings.WithGuard())

bot.On(func(event *starlings.MessageCreate) {
	if err := bot.State.Apply(event); err != nil {
		log.Print(err)
	}
})
```

Starlings Guard records bounded timing and error metrics for manual work. `GuardCompare` can also compare pure manual and automatic implementations for speed and result mismatches. It does not run side effects twice. The full contract is in [the state and Guard guide](docs/state.md).

## Voice

```go
voice, err := bot.ConnectVoice(ctx, guildID, channelID)
if err != nil {
	log.Fatal(err)
}
defer voice.Close(context.Background())

track, err := voice.PlayFile(ctx, "song.mp3")
if err != nil {
	log.Fatal(err)
}
log.Print(track.Wait())
```

FFmpeg supplies the decoder and may read the audio track from a video file. Discord does not let bot accounts publish camera or Go Live video. `OpusProvider` is the lower-level path for applications that already produce 48 kHz Opus frames.

Voice negotiates Discord's current AEAD modes and joins the DAVE MLS group before playback. It can also receive other users' Opus packets. See [`examples/voice`](examples/voice/main.go) and [`examples/soundboard`](examples/soundboard/main.go).

## Starlog

Starlog is optional terminal-native observability:

```go
logs := starlings.NewStarlog("my bot")
bot := starlings.New(token, starlings.WithStarlog(logs))
```

In a terminal it renders a live dashboard with bot and shard status, CPU, heap, goroutines, log rate, scrollback, media activity, and animated Star Pets. Outside a terminal it falls back to a clean stream, so CI and files never receive dashboard control codes.

The pieces are independent. Keep styled streaming logs without the dashboard, select one pet or a crew, send subprocess output through `BuildWriter` and `Writer`, or attach filtered sinks to files and pipes. System media detection supports Windows media sessions and Linux MPRIS, including a Spotify card. The renderer and terminal input are written in Go.

Run the network-free demo to see it without a Discord token:

```sh
go run ./examples/starlogdemo
```

## StarDB

StarDB is an optional package for bots that want one small persistence API:

```go
db, err := stardb.OpenJSON("data/settings.json", stardb.WithLogger(logs.Logger()))
if err != nil {
	log.Fatal(err)
}
defer db.Close()

err = stardb.Save(ctx, db, "guild:"+guildID.String(), settings)
settings, err := stardb.Load[GuildSettings](ctx, db, "guild:"+guildID.String())

settings, err = stardb.Update(ctx, db, "guild:"+guildID.String(), GuildSettings{},
	func(value *GuildSettings) error {
		value.Prefix = "?"
		return nil
	})
```

The same `Store` contract covers atomic JSON and CSV, driver-neutral SQL, Firebase Realtime Database, and Supabase. It includes strict size limits, optional authenticated encryption, parameterised SQL, HTTPS enforcement for hosted backends, and credential-safe logging. See [the StarDB guide](docs/stardb.md).

## Escape hatches

Starlings does not require waiting for a wrapper when Discord adds something:

- `Request` and `RequestRaw` expose REST.
- `SendGateway` and `SendGuildGateway` expose gateway opcodes.
- `OnRaw` receives untyped dispatches.
- `HandleGatewayFrame` supports deterministic replay and custom transports.
- Manual sharding and manual state modes keep fleet and cache ownership with the application.

Typed helpers and escape hatches use the same transport, rate limiter, and state paths.

## Measured against other libraries

The arena replays fixed offline fixtures through public APIs, takes 30 process-isolated samples, and keeps different coverage classes separate. Among the five libraries that completed every state workload on the recorded WSL2/i7-9750H run, Starlings ranked first for permission resolution, second for guild ingestion, and fourth for safe member lookup. Four libraries exposed a comparable full-dispatch path; Starlings ranked first there.

| Workload | Starlings | Fastest other result in the same class |
| --- | ---: | ---: |
| handled message dispatch | 7.615 µs | discord.js 8.387 µs |
| permission resolution | 191.8 ns | DiscordGo 410.1 ns |
| guild state ingestion | 2.098 ms | discord.js 1.003 ms |
| member lookup | 173.2 ns | DiscordGo 117.8 ns |

The lookup result includes Starlings' deep snapshot; faster libraries may return references into mutable cache state. Cold-start numbers cover all 17 pinned libraries but are runtime-load measurements, not a feature ranking. The exact commands, pins, confidence intervals, memory results, coverage table, and limitations are in [the arena report](benchmarks/arena/results.md).

## Failure behaviour

Startup warnings catch intent combinations that would silently starve a handler or blank message content. REST errors preserve Discord's status and error code through `APIError`. `Run` returns `FatalError` when reconnecting cannot help, such as a rejected token or disallowed intents.

Handlers are synchronous and ordered by default. A slow handler therefore delays later events; use a goroutine or `WithAsyncEvents(true)` for independent work. Heartbeats remain on their own goroutine.

## Documentation

- [v0.1.1 changes](CHANGELOG.md)
- [Documentation index](docs/README.md)
- [Getting started](docs/getting-started.md)
- [Interactions](docs/interactions.md)
- [Messages and REST](docs/rest-and-messages.md)
- [Voice and soundboard](docs/voice.md)
- [Runtime, gateway, and sharding](docs/runtime.md)
- [Starlog](docs/starlog.md)
- [Social SDK](docs/social-sdk.md)
- [Moving from DiscordGo](docs/migrating-from-discordgo.md)
- [First bot](examples/firstbot/main.go)
- [State and Guard](docs/state.md)
- [StarDB](docs/stardb.md)
- [Gateway events](docs/gateway-events.md)
- [REST surface](docs/rest-surface.md)
- [Schemas](docs/schemas.md)
- [Enums](docs/enums.md)
- [Protocol research and validation](docs/research.md)
- [Arena methodology and results](benchmarks/arena/README.md)

## License

Starlings is licensed under the [MIT License](LICENSE).

Use it in anything, including closed-source and commercial work. The only
condition is that you keep the copyright notice.
