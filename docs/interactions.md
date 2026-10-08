# Interactions

## Register and publish commands

`Slash` records a definition and handler locally. `SyncCommands` publishes all recorded definitions by replacing the current command set.

```go
bot.Slash("say", "Repeat text", func(i *starlings.InteractionCreate) {
	text := i.String("text")
	if err := i.Reply(text); err != nil {
		log.Print(err)
	}
}, starlings.StringOption("text", "Text to repeat", true))

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

This syncs once after READY. Omit the option when deployment code should own
when command definitions change.

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
	i.EditResponse(ctx, starlings.InteractionResponseData{Content: "failed: " + err.Error()})
	return
}
i.EditResponse(ctx, starlings.InteractionResponseData{Content: result})
```

Do not defer and then call `Reply`; the initial response has already been used.

## Components and modals

Buttons and selects return as `InteractionMessageComponent`; modal submissions return as `InteractionModalSubmit`. Route them by `i.Data.CustomID`.

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
