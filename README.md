# Starlings

<img src="assets/starlings-logo.png" alt="Starlings logo" width="180">

Starlings is a Discord library for Go. It keeps the common bot code short without hiding the gateway, state, voice, or REST API when you need control.

```go
func main() {
	bot := starlings.New()
	bot.Slash("ping", "Is the bot alive?", ping)
	log.Fatal(bot.Run())
}

func ping() string {
	return "Pong!"
}
```

`New` reads the token from `DISCORD_TOKEN`. `Run` chooses intents from your handlers, publishes commands when they change, and handles reconnects, session resumes, gateway compression, and Discord's recommended shard count. A small bot can stay small; a large one does not need a different client, and every automatic step has a switch.

## Start here

Starlings requires Go 1.27 or newer.

```sh
go get github.com/razecrs/starlings@latest
```

Set `DISCORD_TOKEN` (or put it in a `.env` file), set `DISCORD_GUILD_ID` to a server you are testing in so commands appear there at once, then run the first example:

```sh
go run ./examples/ping
```

The token belongs in the environment, never in source. Copy [`.env.example`](.env.example) for the larger examples that load several settings.

## The short path

A handler asks only for what it uses and returns what to send. Options come from a struct:

```go
type BanArgs struct {
	User   *starlings.Member `desc:"Who to ban"`
	Reason string            `desc:"Why" max:"400"`
}

func ban(i *starlings.InteractionCreate, a BanArgs) (string, error) {
	if err := i.Member.CanModerate(a.User); err != nil {
		return "", err
	}
	return "Banned " + a.User.Mention(), a.User.Ban(a.Reason, 0)
}

bot.Slash("ban", "Ban a member", ban).Require(starlings.PermissionBanMembers)
```

`Require` sets Discord's default permissions and checks them again on every use. Errors from `UserErrorf`, refused moderation, and common Discord refusals are shown privately to the user; anything else is logged, and the user sees a generic message. Slow handlers are deferred automatically, so a late reply still arrives.

Messages, members, channels, and users act on themselves (`msg.Reply`, `member.Timeout`, `channel.Send`, `user.Send`). Buttons and modals use the same handler shapes with routes such as `ticket:close:{id}`, and `Pager` builds a paged view that keeps working after a restart. See [the interactions guide](docs/interactions.md) and [messages and REST](docs/rest-and-messages.md).

Prefix commands remain available:

```go
bot := starlings.NewCommandBot()
bot.Command("say", func(m *starlings.MessageCreate, args []string) {
	m.ReplyComplex(starlings.SendData{
		Content:         strings.Join(args, " "),
		AllowedMentions: starlings.NoMentions(),
	})
})
```

`NewCommandBot` selects the three intents prefix commands need and skips the
resource cache they usually do not. Message Content must still be enabled in
Discord's developer portal.

Typed handlers infer the event from the argument:

```go
bot.On(func(r *starlings.Ready) { log.Printf("online as %s", r.User.Tag()) })
bot.On(func(m *starlings.MessageCreate) { /* ... */ })
```

For compile-time handler checks, use `starlings.On(bot, handler)`. `Listen` returns an unsubscribe function and `Once` removes itself after the first event.

Interactions can arrive through the gateway or a verified HTTP endpoint. `InteractionHandler` is a normal `net/http` handler; `VerifyInteraction` is available when the application owns request routing.

For full control, `starlings.New(starlings.Explicit(), starlings.WithToken(token))` turns off everything automatic: inferred intents, command publishing, automatic deferral, panic recovery, and memory trimming. The [runtime guide](docs/runtime.md) lists each behaviour and its own switch.

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
bot := starlings.New(starlings.WithToken(token),
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
conn, err := voice.Connect(ctx, bot, guildID, channelID)
if err != nil {
	log.Fatal(err)
}
defer conn.Close(context.Background())

track, err := conn.PlayFile(ctx, "song.mp3")
if err != nil {
	log.Fatal(err)
}
log.Print(track.Wait())
```

FFmpeg supplies the decoder and may read the audio track from a video file. Discord does not let bot accounts publish camera or Go Live video. `voice.OpusProvider` is the lower-level path for applications that already produce 48 kHz Opus frames.

Voice is the `github.com/razecrs/starlings/voice` package, so bots that do not import it do not link it. It negotiates Discord's current AEAD modes and joins the DAVE MLS group before playback. It can also receive other users' Opus packets. See [`examples/voice`](examples/voice/main.go) and [`examples/soundboard`](examples/soundboard/main.go).

## Starlog

Starlog is optional terminal-native observability:

```go
logs := starlings.NewStarlog("my bot")
bot := starlings.New(starlings.WithToken(token), starlings.WithStarlog(logs))
```

In a terminal it renders a live dashboard with bot and shard status, CPU, heap, goroutines, log rate, scrollback, media activity, and animated Star Pets. Outside a terminal it falls back to a clean stream, so CI and files never receive dashboard control codes.

The pieces are independent. Keep styled streaming logs without the dashboard, select one pet or a crew, send subprocess output through `BuildWriter` and `Writer`, or attach filtered sinks to files and pipes. System media detection (`sysmedia.Option()`) supports Windows media sessions and Linux MPRIS, including a Spotify card; the pixel-art pets come from `import _ "github.com/razecrs/starlings/starpets"`. Both are separate packages, so bots that do not use them do not link them. The renderer and terminal input are written in Go.

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

Startup warnings catch intent combinations that would silently starve a handler or blank message content. REST errors preserve Discord's status and error code through `APIError`, and common codes carry a plain explanation. `Run` returns `FatalError` when reconnecting cannot help, such as a rejected token or disallowed intents. After Discord rejects the token, REST calls stop instead of adding to Cloudflare's invalid-request count.

Handlers are synchronous and ordered by default. A slow handler therefore delays later events; use a goroutine or `WithAsyncEvents(true)` for independent work. Heartbeats remain on their own goroutine. A panic in a handler is logged with its stack and the bot keeps running, unless `WithPanicRecovery(false)` is set.

## Documentation

- [Changes](CHANGELOG.md)
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
