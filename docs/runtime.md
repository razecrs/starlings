# Runtime, gateway, and sharding

## Connection lifecycle

`RunContext` owns identify, heartbeat, zombie detection, resume, reconnect backoff, compression, and shutdown. A recoverable disconnect stays inside `RunContext`. A rejected token, disallowed intent, or other non-retryable close returns a `*FatalError`.

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

if err := bot.RunContext(ctx); err != nil {
	var fatal *starlings.FatalError
	if errors.As(err, &fatal) {
		log.Fatalf("gateway configuration failed: %v", fatal)
	}
	log.Fatal(err)
}
```

Gateway compression and automatic sharding are enabled by default. Disable compression only for a debugging proxy that needs plain frames.

While asking Discord for the shard count, the client already opens the gateway
connection at the usual address, and identifies only after the answer
confirms one shard there. A single-shard bot sends IDENTIFY about 200 ms
sooner. Gateway commands are paced to Discord's limit of 120 per 60 seconds
per connection, with a reserve kept for heartbeats, identify, and resume.

`WaitReady` waits for the first READY only. `Online` and `ShardStatus.Ready`
follow the live session and become false while a shard is disconnected. The
`Disconnect` event carries Discord's `CloseCode` and `CloseReason`; a close
caused by something the bot sends, such as 4002, is explained in plain words
and logged once as an error if it repeats after every reconnect.

## What happens automatically

Each automatic behaviour has its own switch. `Explicit` turns them all off, for
a client that does only what it is configured to do; options after it can turn
single behaviours back on.

| Behaviour | Default | Switch |
| --- | --- | --- |
| Token from `DISCORD_TOKEN` or `.env` | on | `WithToken` |
| Intents chosen from registered handlers | on when `WithIntents` is not used | `WithIntents` |
| Publishing changed slash commands after READY | on | `WithAutoSync(false)`, `WithCommandSync(guild)` |
| Deferring slow interaction handlers after 2.2 s | on | `WithAutoDefer(0)`, `.NoAutoDefer()` |
| Recovering and logging handler panics | on | `WithPanicRecovery(false)` |
| Returning startup memory to the OS | on | `WithMemoryTrim(false)` |
| Pacing to the global REST limit | 50/s | `WithGlobalRateLimit(n)` |
| Bounding REST calls without a deadline | 1 minute | `WithRequestTimeout(d)` |

```go
bot := starlings.New(starlings.Explicit(),
	starlings.WithToken(token),
	starlings.WithIntents(starlings.IntentGuilds|starlings.IntentGuildMessages),
	starlings.WithAutoDefer(2*time.Second)) // back on, just this one
```

When intents are inferred, `Run` logs any privileged ones the handlers need;
those must also be enabled under Bot in the developer portal. With recovery
on, a handler panic is logged with its stack and the bot keeps running, the
way `net/http` treats a panic in one request.

Connecting delivers every guild and member list at once, and the Go runtime
keeps the memory that burst needed unless asked to return it. With trimming
on, the client returns it once, after guild and member events have been quiet
for two seconds.

## Intents are data ownership

Intents control which events Discord sends; `StateConfig` controls which received objects Starlings retains. They solve different problems. A handler can require an event even when its cache category is disabled, and the state cache can require decoding an event even when no application handler exists.

Starlings skips a dispatch payload only when no application handler and no enabled internal consumer needs it. Internal consumers include state maintenance and voice bookkeeping.

Startup warnings identify common dead configurations, such as a `MessageCreate` handler without guild/direct message intents or prefix commands without Message Content.

## Loading complete member lists

READY and `GuildCreate` are not guaranteed to contain every member of a large
guild. With the Guild Members intent enabled, request the remaining chunks:

```go
if err := bot.RequestAllMembers(ctx, guildID, "startup-members"); err != nil {
	log.Print(err)
}

bot.On(func(chunk *starlings.GuildMembersChunk) {
	log.Printf("received chunk %d/%d", chunk.ChunkIndex+1, chunk.ChunkCount)
})
```

The automatic state cache consumes each chunk before the application handler.
Use `RequestMembers` for a name prefix or up to 100 exact user IDs. A query of
`""` with limit `0` means every member; `RequestAllMembers` builds that wire
shape for you.

`WithMemberChunking(true)` does this for every guild whose GUILD_CREATE was
incomplete. Requests go through the gateway command limit, so a bot in many
guilds fills its cache over a few minutes instead of being disconnected.

## Dispatch ordering

State maintenance runs before application handlers in automatic mode. Handlers for one event run in registration order and synchronously by default. This gives deterministic snapshots and natural backpressure, but a blocking handler delays later dispatches.

`WithAsyncEvents(true)` runs independent application handlers concurrently. Internal state mutations remain ordered. Code that depends on handler-to-handler ordering should stay synchronous.

## Automatic and manual sharding

With automatic sharding, Starlings asks Discord for the recommended count and starts the fleet. Guild traffic is routed with Discord's snowflake formula. `ShardIDForGuild` exposes that decision.

Use manual sharding when separate processes or schedulers own the fleet:

```go
bot := starlings.New(starlings.WithToken(token),
	starlings.WithAutoSharding(false),
	starlings.WithShard(shardID, shardCount),
	starlings.WithIntents(intents))
```

Every process must use the same total count, and each shard ID must be unique in `[0, shardCount)`. The application owns process placement and identify concurrency in this mode.

## Raw escape hatches

```go
stop := bot.OnRaw(func(event *starlings.RawEvent) {
	log.Printf("gateway event %s", event.Name)
})
defer stop()

err := bot.SendGuildGateway(ctx, guildID, opcode, payload)
```

`OnRaw` observes dispatches without replacing typed handlers. `SendGateway` broadcasts an opcode; `SendGuildGateway` selects the guild's shard. Raw frame variants expect already encoded frames.

Add-on packages such as `voice` use two hooks that applications can use too.
`OnOrdered` registers a handler that runs on the gateway goroutine, in event
order, before application handlers, even with `WithAsyncEvents`; it must
return quickly. `Client.Extension` keeps one value per client, shared by every
shard, and closes it with the client if it has a `Close(context.Context)`
method.

`HandleGatewayFrame` sends a captured frame through raw handlers, typed dispatch, and internal consumers. It is meant for deterministic replay, tests, and custom transports. Offline dispatch frames are safe to replay; control opcodes retain their live behaviour and should not be injected without a transport.

## State modes

Automatic state is the default. Manual state stops automatic mutation but uses the same `State.Apply` implementation when called by the application. See [state and Guard](state.md) before selecting manual mode; skipping events required by another cached object can make apparently unrelated lookups incomplete.
