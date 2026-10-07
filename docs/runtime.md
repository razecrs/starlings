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

## Dispatch ordering

State maintenance runs before application handlers in automatic mode. Handlers for one event run in registration order and synchronously by default. This gives deterministic snapshots and natural backpressure, but a blocking handler delays later dispatches.

`WithAsyncEvents(true)` runs independent application handlers concurrently. Internal state mutations remain ordered. Code that depends on handler-to-handler ordering should stay synchronous.

## Automatic and manual sharding

With automatic sharding, Starlings asks Discord for the recommended count and starts the fleet. Guild traffic is routed with Discord's snowflake formula. `ShardIDForGuild` exposes that decision.

Use manual sharding when separate processes or schedulers own the fleet:

```go
bot := starlings.New(token,
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

`HandleGatewayFrame` sends a captured frame through raw handlers, typed dispatch, and internal consumers. It is meant for deterministic replay, tests, and custom transports. Offline dispatch frames are safe to replay; control opcodes retain their live behaviour and should not be injected without a transport.

## State modes

Automatic state is the default. Manual state stops automatic mutation but uses the same `State.Apply` implementation when called by the application. See [state and Guard](state.md) before selecting manual mode; skipping events required by another cached object can make apparently unrelated lookups incomplete.
