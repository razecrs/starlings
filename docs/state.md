# State and Starlings Guard

Starlings' default state is complete and automatic. You can use it immediately:

```go
guild, ok := bot.State.Guild(guildID)
message, ok := bot.State.Message(channelID, messageID)
permissions, err := bot.State.ChannelPermissions(channelID, userID)
color, err := bot.State.UserColor(guildID, userID)
```

All returned values are deep snapshots. Changing a returned member's roles,
an embed field, a component, or any other nested value cannot corrupt the
cache or race with the gateway.

## What is tracked

The zero-configuration cache tracks:

- guilds;
- channels and threads;
- members and users;
- roles, guild emoji, and stickers;
- thread members;
- voice states and presences;
- the newest 100 messages in each channel.

`State.Stats()` returns counts without copying the cached objects. Singular
lookups and plural snapshots are available for every category.

Message caches are bounded independently per channel. They retain insertion
order, update edited messages in place, keep reaction counts and pin
timestamps coherent, and attach the cached objects to message delete events.

Update events expose `BeforeUpdate`; delete events expose `BeforeDelete`.
Bulk and whole-set events use slices. The snapshot is assigned before any
application handler runs in automatic mode.

## Configure only what you need

```go
state := starlings.DefaultStateConfig()
state.Presences = false
state.ThreadMembers = false
state.MaxMessagesPerChannel = 25 // zero disables messages

bot := starlings.New(token, starlings.WithStateCache(state))
```

Disabled categories do not retain objects. When an event is not needed by
another enabled category or by an application handler, Starlings does not
decode it.

Effective permission calculation requires guilds, channels, members, and
roles. It applies the guild owner and administrator rules, then the
`@everyone`, combined-role, and member overwrite layers in Discord's order.
Threads inherit their parent's overwrite set. `UserColor` selects the
highest coloured role.

## Manual mode

Manual mode stops Starlings from applying resource events automatically:

```go
bot := starlings.New(token, starlings.WithStateMode(starlings.StateManual))

bot.On(func(event *starlings.ChannelUpdate) {
	// inspect or persist the raw update first, if needed
	if err := bot.State.Apply(event); err != nil {
		log.Print(err)
	}
	// state now contains the update
})
```

`State.Apply` is the same mutation path automatic mode uses. That keeps
manual control from becoming a second, less-tested cache implementation.
`ErrUnsupportedStateEvent` means an event has no cache mutation and can be
ignored by a generic event pipeline.

## Starlings Guard

Enable Guard beside manual mode:

```go
bot := starlings.New(token,
	starlings.WithStateMode(starlings.StateManual),
	starlings.WithGuard())
```

Every manual `State.Apply` is then timed and labelled by event, with no
payload retention and a fixed-size latency histogram.

Use `Measure` when an operation has side effects. It executes the function
once:

```go
err := bot.Guard().Measure("custom-send", starlings.GuardManual, func() error {
	return mySend(ctx, message)
})
```

Use `GuardCompare` only for pure work that is safe to execute twice:

```go
result, err := starlings.GuardCompare(
	bot.Guard(),
	"permission-check",
	func(a, b starlings.Permissions) bool { return a == b },
	manualPermissions,
	automaticPermissions,
)
```

Guard cannot safely discover and replay arbitrary Go code: doing that to a
message send, payment, file write, or moderation action would duplicate the
side effect. Explicit `Measure` calls compare production aggregates without
replaying anything; explicit `GuardCompare` calls add output mismatch checks
where double execution is safe.

`bot.Guard().Report()` returns structured metrics and advice. Its
`String()` method is ready for a development log:

```go
log.Print(bot.Guard().Report())
```

Run the offline showcase to see Guard catch several realistic manual paths:

```sh
go run ./examples/guard
```

It demonstrates a permission mismatch, a slower linear member lookup, a less
stable decoder, and a side effect that is measured exactly once. Use `-plain`
to remove colour when capturing the report in a file or CI log.
