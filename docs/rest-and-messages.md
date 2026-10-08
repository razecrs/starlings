# Messages and REST

## Common message operations

```go
message, err := bot.Send(ctx, channelID, "hello")
message, err = bot.EditMessage(ctx, channelID, message.ID,
	starlings.SendData{Content: "hello again"})
err = bot.React(ctx, channelID, message.ID, starlings.Emoji{Name: "👍"})
err = bot.DeleteMessage(ctx, channelID, message.ID, "cleanup")
```

Event convenience methods (`Reply`, `Send`, `React`, and `Delete`) use the event's client and IDs. Client methods accept a context and are the better fit when the operation belongs to a larger request lifecycle.

## Values that act on themselves

Messages, members, users, channels, roles, and guilds that come from events,
`State`, or REST calls know which client they came from:

```go
msg.Reply("done")
member.Timeout(10*time.Minute, "spam")
user.Send("a direct message")
channel.SendEmbed(starlings.NewEmbed("Build 42"))
```

| Type | Methods |
| --- | --- |
| `Message` | `Reply`, `ReplyComplex`, `ReplyEmbed`, `Edit`, `Delete`, `React`, `Unreact`, `Pin`, `Unpin`, `Link` |
| `Member` | `Ban`, `Kick`, `Timeout`, `ClearTimeout`, `AddRole`, `RemoveRole`, `SetNick`, `Send`, `SendEmbed`, `CanModerate`, `GuildID` |
| `User` | `Send`, `SendComplex`, `SendEmbed` |
| `Channel` | `Send`, `SendComplex`, `SendEmbed`, `Typing`, `Purge`, `Delete` |
| `Role` | `Delete` |
| `Guild` | `Member`, `Ban`, `Unban` |

Each method calls the matching `Client` method, which stays available. A user's
DM channel is created once and reused. The methods use a background context
bounded by `WithRequestTimeout`; `WithContext` returns a copy that uses yours:

```go
err := member.WithContext(ctx).Kick("raid")
```

A value built by hand, such as a struct literal, has no client and returns
`ErrUnbound`. `Sendable` and `Mentionable` describe what can receive a message
or be mentioned.

`FetchMember`, `FetchChannel`, and `FetchGuild` read the cache and fall back to
a REST request on a miss. `State` lookups never make requests, so a network
call is always visible in the name.

## Rich messages and mentions

```go
_, err := bot.SendComplex(ctx, channelID, starlings.SendData{
	Content: "deployment ready",
	Embeds: []starlings.Embed{{
		Title: "Build 42",
		Color: 0x5865F2,
	}},
	Components: []starlings.Component{
		starlings.ActionRow(
			starlings.Button(starlings.ButtonPrimary, "Deploy", "deploy:42"),
			starlings.LinkButton("Logs", logsURL),
		),
	},
	AllowedMentions: starlings.NoMentions(),
})
```

Discord parses mentions by default. Use `NoMentions` for echoed user input, logs, external content, and templates unless the bot intentionally mentions people or roles.

Components V2 use `ComponentsMessage`, `Container`, `Section`, `TextDisplay`, `Thumbnail`, and related builders. Polls start with `NewPoll`; set duration and layout fields before sending.

## Uploads

```go
file, err := starlings.FileFromPath("report.pdf")
if err != nil {
	log.Fatal(err)
}
file.Description = "Build report"

_, err = bot.SendFiles(ctx, channelID,
	starlings.SendData{Content: "report", AllowedMentions: starlings.NoMentions()},
	file)
```

`FileFromPath` streams from disk while Starlings builds the multipart request; it does not retain an open file. `FileFromBytes` is convenient for generated data already in memory. Give every file an extension so Discord can infer previews and content type.

## History and pagination

`MessageHistory` walks a channel from newest to oldest, 100 messages per
request, and stops requesting as soon as the loop ends:

```go
for msg, err := range bot.MessageHistory(ctx, channelID, 0) {
	if err != nil {
		return err
	}
	if msg.Timestamp.Before(cutoff) {
		break
	}
	archive(msg)
}
```

`Messages` accepts `MessagesQuery` for a single before/after/around page. Discord page limits still apply.

`Purge` deletes recent messages, optionally only those matching a function.
Messages under 14 days old are bulk-deleted; older ones, which Discord refuses
in bulk, are deleted one at a time up to `MaxOld`:

```go
res, err := channel.Purge(starlings.PurgeOptions{
	Count:  50,
	Match:  func(m *starlings.Message) bool { return m.Author.ID == spammerID },
	Reason: "spam cleanup",
})
log.Printf("deleted %d of %d matching", res.Deleted, res.Matched)
```

`DownloadAttachment` reads an attachment with a size limit. It only fetches from
Discord's CDN, including after redirects, so a URL taken from user input cannot
make the bot request internal addresses.

## Errors and retries

Starlings handles Discord rate-limit buckets, global rate limits, 429 responses, and bounded retries for transient server failures. Application code should classify the final error:

```go
if _, err := bot.Send(ctx, channelID, "hello"); err != nil {
	switch {
	case starlings.IsUnauthorized(err):
		log.Fatal("the token was rejected")
	case starlings.IsForbidden(err):
		log.Printf("missing permission: %v", err)
	case starlings.IsNotFound(err):
		log.Printf("channel was removed")
	default:
		log.Printf("send failed: %v", err)
	}
}
```

`APIError` preserves the HTTP status, Discord error code, field validation failures, and a bounded raw body. Error strings sanitise untrusted control characters. Do not blindly retry permission, validation, or not-found errors.

Named codes such as `ErrorMissingPermissions`, `ErrorUnknownMember`, and
`ErrorBulkDeleteTooOld` work with `IsDiscordCode(err, codes...)`. For common
codes, `APIError.UserMessage` explains the refusal in words a user can act on;
error-returning interaction handlers show it automatically.

### How rate limits are handled

- A route whose limits are not known yet sends one request first, then as many
  as Discord reports, instead of a burst that earns 429s.
- Routes Discord reports as one bucket share one count. Interaction callbacks
  and webhooks are limited per interaction and per webhook, as Discord counts
  them.
- Bot requests are paced to the global limit of 50 per second before Discord
  answers 429. `WithGlobalRateLimit` changes the rate if Discord granted a
  higher one. Interaction callbacks are exempt, as they are at Discord, and also
  skip `WithPacing`.
- After a 401, further requests return `ErrTokenRejected` without being sent.
  `InvalidRequests` reports the 401, 403, and 429 count that Cloudflare limits
  to 10,000 per ten minutes per IP.

Waiting is the default. For work that is pointless later, such as renaming a
channel inside an interaction, `FailFast` returns a `*RateLimitError` instead:

```go
_, err := bot.ModifyChannel(starlings.FailFast(ctx), channelID, params, "rename")
var limited *starlings.RateLimitError
if errors.As(err, &limited) {
	// retry after limited.RetryAfter, or tell the user
}
```

The `RateLimit` event reports 429 responses; `RateLimitWait` reports time spent
waiting on a bucket Starlings already knew was empty.

## Audit-log reasons

Administrative helpers that can appear in Discord's audit log accept a `reason` argument. Starlings encodes it into `X-Audit-Log-Reason`. Keep it concise and do not put credentials or private user data into it.

## Raw REST

Use the escape hatch for a new Discord route while keeping Starlings' HTTP transport and limiter:

```go
var out MyResponse
err := bot.Request(ctx, starlings.RESTRequest{
	Method: http.MethodGet,
	Path:   "/guilds/" + guildID.String() + "/new-resource",
	Route:  "GET /guilds/" + guildID.String() + "/new-resource",
}, &out)
```

`Route` is the rate-limit bucket identity, not merely a display label. Preserve major parameters such as guild/channel/webhook IDs and replace non-major IDs consistently. `RequestRaw` returns bounded bytes for non-JSON or not-yet-modelled responses. Prefer typed helpers when one exists; the generated [REST surface](rest-surface.md) is the index.
