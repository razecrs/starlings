# Social SDK server APIs

Discord's Social SDK surface uses two visibly separate authorities in Starlings.

- Lobby administration methods on `Client` use the bot token.
- Player actions on `SocialClient` use an OAuth2 bearer token with `sdk.social_layer`.

Starlings never falls back from a missing player token to bot authorization.

## Lobbies

```go
lobby, err := bot.CreateLobby(ctx, starlings.LobbyCreate{
	Metadata: map[string]string{"mode": "ranked"},
})
if err != nil {
	log.Fatal(err)
}

player := bot.Social(os.Getenv("DISCORD_SOCIAL_TOKEN"))
message, err := player.SendLobbyMessage(ctx, lobby.ID, "ready?")
```

The bot client can create, fetch, edit, and delete lobbies; add, bulk-update, and remove members; create linked-channel invites; and attach moderation metadata. Bulk member updates accept 1–25 entries and validate their shape before sending.

The player client owns user-scoped lobby messages and other calls requiring the OAuth grant. Store its access token like any other credential and use a new `SocialClient` when the authorized player changes.

## Application access

These routes require Social SDK access to be enabled for the Discord application. A 401/403 may mean the application lacks access, the OAuth token lacks the scope, the player has not authorized the application, or the bot lacks the relevant lobby authority. Inspect `APIError.Code` rather than treating every forbidden response as the same setup mistake.

## Provisional accounts

The provisional-account exchange and unmerge methods deliberately use their documented confidential-client form and do not attach the bot authorization header. Keep client secrets server-side. Do not send them through Starlog, Discord messages, widget metadata, or a browser bundle.

## Profile widgets and identities

Application Identity Profile routes use application and player IDs plus the provider-issued user ID from your own system. Writes replace the supplied `data` object as a whole, so read/merge first when partial application state must survive.

The widget-config API is less stable than the documented REST surface. [Game Stats Widgets](game-stats-widgets.md) records the observed layouts, field types, asset rules, authorization scope, and testing workflow. Treat undocumented routes as version-sensitive and keep raw fallbacks isolated behind application code.

## Testing

Use a separate development application and synthetic players/lobbies. Never run destructive lobby, identity, or provisional-account tests against production accounts. Transport-level tests can verify headers, validation, and response decoding offline; live end-to-end coverage still depends on an application Discord has enabled for the Social SDK.
