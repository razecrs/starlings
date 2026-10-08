# Interactions

## Register and publish commands

`Slash` records a definition and handler locally. `Run` publishes them after
READY, but only when they differ from what Discord already has. Set
`DISCORD_GUILD_ID` for a development guild; without it, the target is global.
Publishing replaces the command set in that scope. If your own code calls
`SyncCommands` shortly after READY, automatic publishing stands down, so a bot
that manages its own commands is never published somewhere else.
`WithAutoSync(false)` turns automatic publishing off.

Start with the short form:

```go
bot.Slash("ping", "Check the bot", func() string { return "pong" })

type SayArgs struct {
	Text string `desc:"What to say" min:"1" max:"2000"`
}
bot.Slash("say", "Repeat text", func(a SayArgs) starlings.Response {
	return starlings.Ephemeral(a.Text)
})
```

A handler may take `*InteractionCreate`, an argument struct, or both, and
return a string, `*Embed`, `Response`, an error, or a result plus an error.
Fields become options; `optional:""` makes a field optional. Resource fields
such as `*User`, `*Member`, `*Channel`, and `*Attachment` use Discord's resolved
data without a lookup request. The [slashbot example](../examples/slashbot/main.go)
is a complete program.

Field types include `string`, `bool`, every integer and float kind, `Snowflake`,
and `time.Duration`, which users type as `10m`, `2h`, or `3d`. Tags set the
details: `desc`, `name`, `optional`, `min` and `max` (a number range, or text
length), `choices:"Label=value|Label=value"`, `channel:"text,thread"`, and
`autocomplete`. Required options are placed first automatically, and a
definition Discord would reject stops the program at startup.
`ParseDuration` uses the same duration rules.

Return `UserErrorf` for a message safe to show privately to the user. Other
errors are logged and replaced with a generic failure, not sent verbatim.
Common Discord refusals are explained instead: a missing permission, an
unknown member, a message too old to bulk-delete, and others carry a
`UserMessage` the user can act on.
`.Require(perms)` enforces member permissions at runtime as well as setting
Discord's defaults. `.BotNeeds(perms)` checks bot permissions. Neither replaces
the ownership, target hierarchy, or application-specific checks your action needs.

The explicit option builders and callbacks remain available:

```go
bot.Slash("say", "Repeat text", func(i *starlings.InteractionCreate) {
	text := i.String("text")
	if err := i.Reply(text); err != nil {
		log.Print(err)
	}
}, starlings.StringOption("text", "Text to repeat", true))

// Create bot with WithAutoSync(false) when deployment code owns sync.
bot.On(func(*starlings.Ready) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := bot.SyncCommands(ctx, developmentGuildID); err != nil {
		log.Printf("syncing commands: %v", err)
	}
})
```

Small bots can replace the Ready handler with an explicit startup option:

```go
bot := starlings.New(starlings.WithToken(token), starlings.WithCommandSync(developmentGuildID))
```

This explicitly selects the sync scope and syncs once after READY. To manage
publication yourself, use `WithAutoSync(false)` and call `SyncCommands`.

A non-zero guild ID updates that guild immediately. Zero publishes global commands, which Discord may take longer to propagate. `SlashCommand` accepts a complete `ApplicationCommand` for context menus, localisations, default permissions, subcommands, and other fields the shorthand does not expose.

## Read options

`InteractionData.Option` searches through subcommands. Scalar accessors return their zero value for an absent or wrong type; validate when zero is meaningful to the command.

```go
name := i.String("name")
count := i.Int("count")
public := i.Bool("public")
user := i.UserOption("member")
channel := i.ChannelOption("channel")
role := i.RoleOption("role")
```

Resolved user, channel, and role helpers use the objects Discord includes in the interaction and do not make a REST request.

Autocomplete has its own handler, so command side effects never run while
someone is typing. `Focused` returns the option being typed:

```go
type RemoveArgs struct {
	Warning string `desc:"Warning" autocomplete:""`
}
bot.Slash("warning-remove", "Delete a warning", removeWarning).
	Autocomplete(func(i *starlings.InteractionCreate) {
		typed := i.Focused().String()
		i.Autocomplete(matchingWarnings(i.GuildID, typed)...)
	})
```

## Respond once, then follow up

An interaction gets one initial acknowledgement. Use one of:

- `Reply`, `ReplyEphemeral`, `ReplyComplex`, or `ReplyFiles` for an immediate response.
- `Defer(false)` or `Defer(true)` when work will take longer.
- `Autocomplete` for an autocomplete request.
- `Modal` to open a form.
- `UpdateMessage` for a component that edits its source message.

After replying or deferring, use `Followup`, `EditResponse`, `DeleteResponse`, and their file variants.

```go
if err := i.Defer(true); err != nil {
	return
}

result, err := doWork(ctx)
if err != nil {
	log.Print(err)
	i.EditResponse(ctx, starlings.InteractionResponseData{Content: "That failed. Please try again later."})
	return
}
i.EditResponse(ctx, starlings.InteractionResponseData{Content: result})
```

After an explicit `Defer`, use `EditResponse` or `Followup`. Automatic deferral
is different: gateway routes default to a 2.2-second timer, and a later `Reply`
is converted to an edit. This is a best-effort acknowledgement, not a guarantee
against network outages or a blocked gateway reader. `WithAutoDefer(0)` disables
it; `.NoAutoDefer()` disables it for one slash command. Open modals promptly:
Discord does not allow opening one after acknowledging. HTTP handlers must
acknowledge before returning; the gateway timer does not apply there.

Embeds have a builder that shortens text to Discord's limits, so a long
user-supplied value cannot make the whole response fail. Return the embed, or
send it explicitly:

```go
e := starlings.NewEmbed("Warning issued").
	SetDescription(reason).
	SetColor(starlings.ColorYellow).
	AddField("Moderator", i.Invoker().Mention(), true)
return e // from a handler, or:
i.ReplyEmbedEphemeral(e)
```

`Answered` and `Deferred` report what the initial response was.

For bounded background work, `SlashTask` acknowledges immediately and runs with
a deadline and shutdown cancellation. Ordinary synchronous handlers still block
the dispatcher, and `WithAsyncEvents(true)` does not impose a concurrency limit.

## Components and modals

Buttons and selects return as `InteractionMessageComponent`; modal submissions
return as `InteractionModalSubmit`. Prefer named routes over a growing switch:

```go
bot.Button("ticket:close:{id}", func(i *starlings.InteractionCreate, p struct{ ID int64 }) (string, error) {
	// Check ownership/permissions and close ticket p.ID here.
	return "closed", nil
})
bot.Modal("ticket:open", func(p struct{ Subject string }) string {
	return "received: " + p.Subject
})
id := starlings.CustomID("ticket", "close", ticketID)
```

Route parameters are untrusted input. Typed decoding rejects malformed numbers,
but does not authorize an action. `CustomID` escapes values, so a value
containing `:` cannot add segments. Inside a handler, `Param`, `ParamID`, and
`ParamInt` read the captured values directly. Stable IDs and registered routes
work after a restart; an in-memory `Watch` does not.

Modal argument structs are filled from text inputs and from the first choice
of each select, matched by custom ID. Fields are named after the custom ID in
snake_case, or by a `name` tag.

`Pager` is a paged view with previous and next buttons. It keeps no state: the
page, the owner, and an argument travel in the custom IDs, so pages work after
a restart, and only the person who opened the view can turn them.

```go
warnings := bot.Pager("warnings", func(i *starlings.InteractionCreate, page int, userID string) (starlings.Page, error) {
	rows, total := loadWarnings(userID, page*5, 5)
	return starlings.Page{Embed: renderWarnings(rows), Pages: (total + 4) / 5}, nil
})

bot.Slash("warnings", "Show a member's warnings", func(i *starlings.InteractionCreate, a struct{ User *starlings.User }) error {
	return warnings.Show(i, a.User.ID.String(), true)
})
```

`Page.Extra` adds components under the arrows, and `Pager.Turn` redraws a page
in place, for example after a button on the page deleted an entry.

For direct event ownership, the original interface still works:

```go
bot.On(func(i *starlings.InteractionCreate) {
	if i.Type != starlings.InteractionMessageComponent {
		return
	}
	switch i.Data.CustomID {
	case "deploy":
		i.UpdateMessage(starlings.InteractionResponseData{Content: "deploying"})
	case "settings":
		i.Modal("settings-form", "Settings",
			starlings.ActionRow(starlings.TextInput(
				starlings.TextInputShort, "prefix", "Command prefix", true)))
	}
})
```

Custom IDs are application-controlled routing data. Keep them short, version them if their format may change, and never place credentials in them.

## Gateway or HTTP delivery

Gateway delivery needs `bot.Run`. HTTP delivery does not need a gateway connection for the request itself:

```go
handler, err := bot.InteractionHandler(os.Getenv("DISCORD_PUBLIC_KEY"))
if err != nil {
	log.Fatal(err)
}

mux := http.NewServeMux()
mux.Handle("/interactions", handler)
server := &http.Server{
	Addr:              ":8080",
	Handler:           mux,
	ReadHeaderTimeout: 5 * time.Second,
	ReadTimeout:       10 * time.Second,
	WriteTimeout:      10 * time.Second,
	IdleTimeout:       60 * time.Second,
}
log.Fatal(server.ListenAndServe())
```

The handler verifies Discord's Ed25519 signature, timestamp, replay window, body limit, and validation ping. Use `VerifyInteraction` when another framework owns routing. Never put an unverified body into the command router.
