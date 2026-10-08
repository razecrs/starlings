# Getting started

## Create the application

Create an application in Discord's developer portal, add a bot, and copy the bot token into an environment variable. Never place it in source or commit a `.env` file.

For a prefix command, enable **Message Content Intent** in the portal. The intent must also be present in code; these are separate switches.

```sh
go mod init example.com/mybot
go get github.com/razecrs/starlings@latest
```

## Run a slash-command bot

Since v0.2.0, `New` takes only options: `New(WithToken(token), options...)`,
or just `New()` when `DISCORD_TOKEN` is set. v0.1 code used
`New(token, options...)`; the [changelog](../CHANGELOG.md) lists every renamed
API.

Set `DISCORD_TOKEN` and `DISCORD_GUILD_ID` in your environment or a local `.env`
file. The guild ID keeps command publishing scoped to your development server.
Without it, automatic sync targets the application's global commands.

```go
package main

import (
	"log"
	"github.com/razecrs/starlings"
)

func main() {
	bot := starlings.New(starlings.WithStateCache(starlings.MinimalStateConfig()))
	bot.Slash("ping", "Check the bot is alive", func() string { return "pong" })
	if err := bot.Run(); err != nil {
		log.Fatal(err)
	}
}
```

Slash commands do not need Message Content intent. `Run` infers intents from
handlers and automatically syncs registered commands after READY, unless
disabled with `WithAutoSync(false)`. Sync replaces the command set in its
target scope: do not let multiple deployments publish different command sets
to the same application and scope.

See [interactions](interactions.md) for typed arguments, private replies,
buttons, errors, and the explicit API.

## Run a prefix-command bot

```go
package main

import (
	"log"

	"github.com/razecrs/starlings"
)

func main() {
	bot := starlings.NewCommandBot()

	bot.Command("ping", func(m *starlings.MessageCreate, _ []string) {
		if _, err := m.Reply("pong"); err != nil {
			log.Print(err)
		}
	})

	if err := bot.Run(); err != nil {
		log.Fatal(err)
	}
}
```

`New` performs no network work. Register commands and handlers first; `Run` connects and blocks until Ctrl-C/SIGTERM or a fatal configuration error. Network drops, resumable sessions, invalid sessions, and Discord-requested reconnects are handled internally.

`NewCommandBot` is `New` with the message intents selected and resource cache
disabled. It does not hide a second client or runtime. Pass `WithStateCache`
or `WithIntents` to replace either preset, or use `New` for the complete
default cache.

## Events and commands

`Command` is for prefix commands. The default prefix is `!`; change it with `WithPrefix`.

```go
bot.Command("say", func(m *starlings.MessageCreate, args []string) {
	if len(args) == 0 {
		m.Reply("say what?")
		return
	}
	m.ReplyComplex(starlings.SendData{
		Content:         strings.Join(args, " "),
		AllowedMentions: starlings.NoMentions(),
	})
})
```

`On` infers a gateway event from the handler argument. The generic form catches a wrong handler type at compile time.

```go
starlings.On(bot, func(r *starlings.Ready) {
	log.Printf("online as %s", r.User.Tag())
})

stop := starlings.Listen(bot, func(m *starlings.MessageCreate) {
	log.Printf("message %s", m.ID)
})
defer stop()
```

Handlers are ordered and synchronous by default. Do not wait on slow I/O inside a handler; launch a goroutine or use `WithAsyncEvents(true)` when handlers are independent. Voice connection setup must run outside the gateway handler because it waits for more gateway events.

## IDs and contexts

IDs received from Discord are already `Snowflake` values. Parse configuration at startup:

```go
channelID, err := starlings.ParseSnowflake(os.Getenv("CHANNEL_ID"))
if err != nil {
	log.Fatal(err)
}
```

Give REST, voice, and startup work deadlines:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if _, err := bot.Send(ctx, channelID, "online"); err != nil {
	log.Print(err)
}
```

## Readiness and shutdown

Calls that depend on the application ID must run after READY. Use a `Ready` handler or `WaitReady`.

```go
go func() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := bot.WaitReady(ctx); err != nil {
		log.Print(err)
		return
	}
	log.Printf("application %s is ready", bot.ApplicationID())
}()
```

`Run` installs signal handling. Services that already own shutdown should call `RunContext`; cancelling its context closes gateway and voice work. `Close` is safe to call more than once.

## Before deploying

- Request only the intents the bot uses and enable privileged ones in the portal.
- Suppress mentions whenever user input can reach a message.
- Put deadlines on external work.
- Log and classify REST errors rather than retrying everything yourself.
- Use guild-scoped slash commands during development, then sync globally for release.
- Run `go test -race ./...` for the bot as well as its normal tests.
