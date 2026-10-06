# Game Stats Widgets - the whole API

The cards a game can put on a Discord user's profile. Two APIs sit behind them:

- **Application Identity Profile** - officially documented, v10. The per-player
  stats.
- **Widget configs** - *not* documented anywhere. The layout. Recovered by
  reading a live published config off the API.

Written down because the undocumented half can disappear without notice, and
because the working example it was recovered from is a personal app that may be
deleted.

Sources: [Game Stats Widgets](https://docs.discord.com/developers/social-layer/game-stats-widgets/overview),
[Application Identity Profile Resource](https://docs.discord.com/developers/resources/application-identity-profile),
[Configuring Your Widget](https://docs.discord.com/developers/social-layer/game-stats-widgets/widget-configuration).

## The gates, in the order they bite

1. **Social SDK enabled** on the application, terms agreed.
2. **The game is claimed.** This is the hard one:
   - a public **Steam** store page (no other storefront is accepted),
   - carrying a visible Discord invite,
   - and *"Discord must have an official record of your game"*, which happens
     only after enough detected playtime across Discord users.
   - Bots and unreleased projects **cannot** be claimed.
3. **The player has authorised the app** - see the scope trap below.

## The scope trap

The identity endpoints require the user to have granted
`application_identities.write`.

**That scope cannot be requested directly.** It is not in Discord's public
OAuth2 scope table, and naming it in an authorize URL returns:

```
Denied - invalid_scope: The requested scope is invalid, unknown, or malformed.
```

It is granted *as part of* the Social SDK scopes. Ask for these instead:

| scope | grants |
| --- | --- |
| `openid sdk.social_layer_presence` | profile data - enough for widgets |
| `openid sdk.social_layer` | the above plus limited-access comms features |

Those Social SDK scopes are themselves only available to applications with the
Social SDK enabled.

## Observed error codes

| code | HTTP | meaning |
| --- | --- | --- |
| `40128` | 403 | *This action requires a claimed game on the application team* - fires on **POST** `/widget-configs` when attempting to publish custom widget layouts via the undocumented endpoint. |
| `50025` | 403 | *Invalid OAuth2 access token* - the target user has not completed the OAuth2 authorization code exchange (or token grant is missing). This occurs when the exchange fails (e.g. missing `client_secret`) or has not been run. Once the OAuth code exchange succeeds with `openid sdk.social_layer_presence`, 50025 is resolved. |
| `50035` | 400 | *Invalid Form Body / APPLICATION_IDENTITY_PROVIDER_USER_ID_MISMATCH* - if an Application Identity already exists for the user, subsequent profile updates must use the existing `provider_issued_user_id` (e.g. `raze` instead of `1`). |
| `invalid_client` | 401 | token exchange without a valid `client_secret` at `POST /oauth2/token`. |

## Application Identity Profile API (documented, v10)

All calls authenticate with a **bot token**. The target user must have granted
the scope above.

| method | path |
| --- | --- |
| `PATCH` | `/applications/{application_id}/users/{user_id}/identities/{provider_issued_user_id}/profile` |
| `GET` | `/applications/{application_id}/users/{user_id}/identities/{provider_issued_user_id}/profile` |
| `GET` | `/users/{user_id}/application-identities/{application_id}` |
| `GET` | `/applications/{application_id}/application-identities/{provider_type}/{provider_issued_user_id}` |
| `POST` | `/users/{user_id}/application-identities/{application_id}/{provider_type}/{provider_issued_user_id}/delete` |

`provider_issued_user_id` is the player's ID **in your own system**, not a
provider enum. The first successful write for a user creates an identity with
provider type `NONE`; later writes must reuse an ID the user already has.

**`data` is replaced wholesale on every PATCH.** Fields you omit are erased.
Omitting `data` entirely leaves the stored stats untouched.

### Profile body

```json
{
  "username": "player-one",
  "metadata": null,
  "data": {
    "primary": { "rank_name": "Silver", "total_wins": 57 },
    "dynamic": [
      { "type": 1, "name": "clan", "value": "Starlings" },
      { "type": 2, "name": "commits", "value": 12847 },
      { "type": 3, "name": "badge", "value": { "url": "https://example.com/b.png" } }
    ]
  }
}
```

Dynamic field types: **1** string, **2** number, **3** media (`{"url": ...}`).
Media URLs are fetched by Discord's servers, so they must be publicly
reachable - no localhost.

### Primary field keys

`season`, `rank_name`, `rank_image`, `highest_rank`, `highest_rank_image`,
`featured_played_character`, `featured_played_character_image`,
`playtime_hours` (decimals allowed), `total_wins`, `current_period_wins`,
`total_games`, `current_period_games`, `total_kills`, `current_period_kills`,
`total_assists`, `current_period_assists`, `total_deaths`,
`current_period_deaths`.

The `*_image` ones take a media object.

## Widget config API (undocumented)

Discord's docs say layouts are configured in the portal editor and describe no
API. These endpoints exist and work:

| method | path | notes |
| --- | --- | --- |
| `GET` | `/applications/{app_id}/widget-configs` | **works cross-application** - any bot token can read any app's published config |
| `POST` | `/applications/{app_id}/widget-configs` | body `{"display_name": "..."}` → `{"config_id": ...}`; gated on `40128` |
| `PATCH` | `/applications/{app_id}/widget-configs/{config_id}` | body `{"surfaces": {...}}` |
| `POST` | `/applications/{app_id}/widget-configs/{config_id}/publish` | |

The cross-application GET is how the schema below was recovered: point a bot
token at any app that has a published widget.

### Config shape

```json
{
  "config_id": "100000000000000001",
  "application_id": "100000000000000002",
  "display_name": "starship",
  "status": "published",
  "published_at": "2026-06-12T17:43:48.449298+00:00",
  "updated_at":   "2026-06-12T17:43:48.449303+00:00",
  "surfaces": { "widget_top": {...}, "widget_bottom": {...}, "add_widget_preview": {...} },
  "resolved_assets": [
    {
      "key": "starship_logo",
      "asset_id": "100000000000000003",
      "asset_type": "image",
      "visibility": "public",
      "metadata": { "width": 512, "height": 512, "content_type": "image/png", "is_animated": false }
    }
  ],
  "application": { "id": "...", "name": "starship", "icon": null, "is_verified": true }
}
```

### Surfaces

Five exist; three are required before a config may be published.

| key | where it renders | required |
| --- | --- | --- |
| `widget_top` | top half of the card on the full profile | yes |
| `widget_bottom` | bottom half of the card | yes |
| `add_widget_preview` | the Add Profile Widget modal | yes |
| `mini_profile` | mini profile popout, only if it is the top widget | no |
| `activity_accessory` | activity card while actively playing | no |

### Layouts and their components

Recovered by reading live published configs off the API.

| layout | surface | components | fields per component |
| --- | --- | --- | --- |
| `widget_top_hero` | `widget_top` | `hero_image` | `image` |
| | | `title` | `text` |
| | | `subtitle_1`, `subtitle_2`, `subtitle_3` | `text`, **`icon`** |
| `widget_bottom_stats` | `widget_bottom` | `stat_1` … `stat_6` | `value`, `label` |
| `widget_bottom_collection` | `widget_bottom` | `item_1` … `item_4` | `image`, `name`, `description` |
| `add_widget_preview_hero` | `add_widget_preview` | `hero_image` | `image` |
| `mini_profile_hero_stat` | `mini_profile` | `hero_image` | `image` |
| | | `stat` | `icon`, `text` |

Only `hero_image` and `title` are required on `widget_top_hero`; the subtitles
are optional, and each carries its own optional `icon` image alongside its
text - that is how a subtitle line gets a small glyph next to it.

Two things worth knowing:

- **Six stat slots** is the tell that distinguishes a widget from an
  Application Role Connection card, which caps at five metadata records.
- **`widget_bottom_collection` is the tile layout** - four entries, each an
  image with a name and a description. This is what a "showcase" style card
  uses instead of a stats grid, and nothing in the SDK or the public docs
  mentions it.

### Field shape

Every field is the same three keys:

```json
{ "value_type": "data", "presentation_type": "number", "value": "total_kills" }
```

| `value_type` | meaning |
| --- | --- |
| `data` | pull from the player's profile; `value` is the **key** (a primary field name, or a dynamic field's `name`). The portal calls this "User Data". |
| `custom_string` | a literal, same for every player. Max 256 chars. |
| `application_asset` | a static image uploaded to the portal; `value` is the asset key. Must be set **Public** or it renders as a skeleton. |

| `presentation_type` | meaning |
| --- | --- |
| `text` | as-is; numbers are not rendered |
| `number` | compact notation, `1500` → `1.5K` |
| `duration` | value read as milliseconds, `23400000` → `6h 30m` |
| `image` | image fields only; has no other presentation type |

`number` and `duration` accept only `data`. `text` accepts `data` or
`custom_string`. Image fields accept `data` (a media field) or
`application_asset`.

Asset keys: max 50 chars, letters, numbers, underscores and hyphens.

### Published config shape

A representative config binds the bottom row to primary fields and uses
static assets for both images:

```json
"surfaces": {
  "widget_top": {
    "layout": "widget_top_hero",
    "components": {
      "title":      { "fields": { "text":  { "value_type": "custom_string",     "presentation_type": "text",  "value": "Starship" } } },
      "hero_image": { "fields": { "image": { "value_type": "application_asset", "presentation_type": "image", "value": "starship_logo" } } }
    }
  },
  "widget_bottom": {
    "layout": "widget_bottom_stats",
    "components": {
      "stat_1": { "fields": { "value": { "value_type": "data", "presentation_type": "number",   "value": "total_kills" },
                              "label": { "value_type": "custom_string", "presentation_type": "text", "value": "Kills" } } },
      "stat_2": { "fields": { "value": { "value_type": "data", "presentation_type": "number",   "value": "total_wins" },
                              "label": { "value_type": "custom_string", "presentation_type": "text", "value": "Wins" } } },
      "stat_3": { "fields": { "value": { "value_type": "data", "presentation_type": "number",   "value": "total_games" },
                              "label": { "value_type": "custom_string", "presentation_type": "text", "value": "Matches" } } },
      "stat_4": { "fields": { "value": { "value_type": "data", "presentation_type": "duration", "value": "playtime_hours" },
                              "label": { "value_type": "custom_string", "presentation_type": "text", "value": "Playtime" } } },
      "stat_5": { "fields": { "value": { "value_type": "data", "presentation_type": "text",     "value": "rank_name" },
                              "label": { "value_type": "custom_string", "presentation_type": "text", "value": "Rank" } } },
      "stat_6": { "fields": { "value": { "value_type": "custom_string", "presentation_type": "text", "value": "season" },
                              "label": { "value_type": "custom_string", "presentation_type": "text", "value": "Season" } } }
    }
  },
  "add_widget_preview": {
    "layout": "add_widget_preview_hero",
    "components": { "hero_image": { "fields": { "image": { "value_type": "application_asset", "presentation_type": "image", "value": "starship_logo" } } } }
  }
}
```

`stat_6` uses `custom_string` with the literal `"season"` rather than binding
to the `season` primary key. This shows that a literal is accepted in a value
slot.

## Testing without publishing

Members of the application's **developer team** can add an unpublished widget
to their own profile. Enable Developer Mode, pick a layout for the three
required surfaces, then use Add Widget on your own profile. Widget configs are
cached per client session, so `Ctrl+R` in Discord after portal changes.

### A collection-layout config

`widget_bottom_collection` creates a showcase card. Each item is three fields:

```json
"widget_bottom": {
  "layout": "widget_bottom_collection",
  "components": {
    "item_1": {
      "fields": {
        "image":       { "value_type": "application_asset", "presentation_type": "image", "value": "project_icon" },
        "name":        { "value_type": "custom_string",     "presentation_type": "text",  "value": "Starship" },
        "description": { "value_type": "custom_string",     "presentation_type": "text",  "value": "A sample project." }
      }
    }
  }
}
```

Its `widget_top_hero` also shows the subtitle `icon` field in use, and its
`mini_profile` surface uses `mini_profile_hero_stat`:

```json
"subtitle_2": {
  "fields": {
    "text": { "value_type": "custom_string",     "presentation_type": "text",  "value": "Developer & Bug Hunter" },
    "icon": { "value_type": "application_asset", "presentation_type": "image", "value": "profile_icon" }
  }
}
```

Asset keys are free-form. Animated GIFs and WebP are accepted;
`resolved_assets` reports `is_animated` and `content_type` per asset.
