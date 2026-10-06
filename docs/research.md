# Discord API research

> Historical audit: the gap counts and work order below describe the initial
> implementation, not the current library. The generated coverage report is
> authoritative; it currently reports 246/246 REST operations. The Social SDK
> and partner server surface was added after this initial audit.
> The component, gateway-opcode, state, voice, and subscription gaps identified
> here have since been implemented.

Written after starlings was already started, which was the wrong order. Building
first and checking the docs later produced code that was confidently wrong in
places - see [What this caught](#what-this-caught). This file is the map that
should have come first.

## Files

| file | what it is | generated? |
| --- | --- | --- |
| [rest-surface.md](rest-surface.md) | all 246 REST operations, by resource | yes |
| [schemas.md](schemas.md) | all 447 object schemas and their fields | yes |
| [enums.md](enums.md) | all 91 enumerations with exact wire values | yes |
| [coverage.md](coverage.md) | which REST operations starlings implements | yes |
| [gateway-events.md](gateway-events.md) | opcodes, close codes, intents, all 88 dispatch events | hand-written |
| [game-stats-widgets.md](game-stats-widgets.md) | profile widgets: the identity API, the undocumented widget-config API, layouts and field vocabulary | hand-written |
| research.md | this file | hand-written |

Regenerate the first four with:

```
cd starlings
go run ./internal/apidoc
```

## Method

**REST and the object model come from Discord's own machine-readable spec**,
not from reading documentation pages. `discord/discord-api-spec` publishes an
OpenAPI 3.1 description of the v10 API; `internal/apidoc` downloads it and
renders the markdown. Transcribing 246 endpoints and 538 schemas by hand would
have introduced errors at a rate that makes the exercise pointless.

**The gateway is not in that spec**, so [gateway-events.md](gateway-events.md)
is transcribed from the documentation - but cross-checked against the spec's
`ActionTypes` enum, which lists every dispatch event name. That check was worth
doing: the enum carries events the prose docs omit entirely (the Social SDK
lobby and `GAME_DIRECT_MESSAGE_*` families, `QUEST_USER_ENROLLMENT`,
`RATE_LIMITED`).

**Coverage is measured, not claimed.** Every REST call in starlings carries a
`Route` field naming its rate-limit bucket. `internal/apidoc` parses those out
of the AST with `go/ast`, normalises both sides so `/channels/{channel_id}`
and `"/channels/" + id.String()` compare equal, and diffs them against the
spec. A route that is not implemented cannot be reported as implemented,
because nothing hand-maintains the list.

The same pass reports **routes starlings declares that the spec does not have**.
That is the valuable direction: a typo in a bucket key does not fail a build or
a test, it just silently shares a rate-limit bucket with the wrong endpoint.

## The size of the surface

| | Discord | starlings | |
| --- | --- | --- | --- |
| REST operations | 246 | 13 | 5.3% |
| Object schemas | 447 | 29 | 6.5% |
| Enumerations | 91 | 14 | 15% |
| Gateway dispatch events | 88 | 20 | 23% |
| Gateway opcodes | 13 | 9 | 69% |
| Voice opcodes | 23 | 0 | |

starlings declares 56 exported struct types, but 20 of those are event wrappers and
7 are Starlings' own (`Client`, `APIError`, `SendData` and friends), leaving 29
that model a Discord object.

The largest untouched areas, by operation count:

| area | operations |
| --- | --- |
| guild scheduled events | 11 |
| guild members | 9 |
| application commands (guild) | 9 |
| invites | 9 |
| roles | 7 |
| webhooks | 7 |
| thread members | 6 |
| guild templates | 6 |
| application commands (global) | 6 |
| lobbies + members + messages | 15 |

## What this caught

Four things in already-written code, none of which a compiler or a test would
have found:

1. **Component types were 65% incomplete.** `component.go` declares types 1–8.
   Discord has 23. Everything Components V2 added - `SECTION` (9),
   `TEXT_DISPLAY` (10), `THUMBNAIL` (11), `MEDIA_GALLERY` (12), `FILE` (13),
   `SEPARATOR` (14), `CONTAINER` (17), `LABEL` (18), `FILE_UPLOAD` (19),
   `RADIO_GROUP` (21), `CHECKBOX_GROUP` (22), `CHECKBOX` (23) - was missing.
   This is the clearest argument for the research-first order: the numbers were
   written from memory and memory was three years stale.

2. **Two rate-limit bucket keys were wrong.** `React` and `Unreact` declared
   `.../messages/{id}/reactions`, but the real endpoint is
   `.../messages/{id}/reactions/{emoji}/@me`. The `PUT` form matched no Discord
   route at all; worse, the `DELETE` form *did* match a different real endpoint
   (delete-all-reactions), so the two would have shared a bucket and throttled
   each other. Found by the orphan check, fixed, now zero orphans.

3. **`MESSAGE_REACTION_REMOVE_EMOJI` was missing** from an otherwise complete
   reaction family - the kind of gap that only shows up against a full list.

4. **Four send opcodes are unimplemented**: Voice State Update (4), Request
   Guild Members (8), Request Soundboard Sounds (31), Request Channel Info
   (43). Opcode 8 in particular is how a bot fetches members of a large guild;
   without it `GuildMembers` cannot work properly at scale.

Things the research **confirmed correct**, which is also worth recording:

- All 21 intent bit positions, including the unassigned gaps at 17–19 and
  22–23.
- All 14 close codes and, more importantly, which six are non-resumable.
  Starlings' `Fatal()` set matches Discord's `reconnect: false` set exactly.
- The snowflake epoch and the 22-bit timestamp shift.

## Design consequences

**Don't hand-write enums.** 91 enumerations, several with 20+ members that
change as Discord ships features. These should be generated from
[enums.md](enums.md)'s source rather than typed. That is the next piece of
tooling: extend `internal/apidoc` to emit Go constants.

**Don't hand-write the object model either, entirely.** 447 schemas is far past
what is worth maintaining by hand, but a fully generated model produces
unpleasant Go - Discord's spec splits nearly every object into `Request` and
`Response` variants and leans on `anyOf` unions. The plan is a hybrid: generate
the wide, mechanical types and enums; hand-write the ~30 objects that carry
convenience methods and deserve good names.

**`Component` needs to become a union.** One flat struct with every field was
defensible for 8 component types. At 23, with nested containers and sections,
it stops being readable. This is the one type where the research changes the
design rather than just the contents.

**Coverage belongs in CI.** The generator already fails loudly on orphan
routes; it should run on every change so a bad bucket key is caught the day it
is written.

## Order of work

Sequenced by how many real bots are blocked by each.

1. **Interactions and application commands.** 12 REST operations plus the
   response callbacks. Nothing modern can be built without them, and the local
   kitty bot needs exactly this.
2. **Components V2.** Redo `Component` as a union with all 23 types.
3. **The rest of the message and channel surface.** 17 message operations, plus
   pins, threads and thread members.
4. **Members, roles, bans, invites, webhooks.** The moderation and
   administration bread and butter, ~30 operations.
5. **Remaining gateway events.** 68 missing; most are small structs, and the
   dispatch machinery already handles them once declared.
6. **Generated enums and wide types**, once the hand-written core has settled
   enough to know what shape the generator should target.
7. **Voice.** A separate websocket, its own 23 opcodes, plus Opus and the DAVE
   encryption protocol. Large and self-contained; last.

The initial audit left Social SDK lobby, `GAME_DIRECT_MESSAGE_*`, `partner-sdk/*`,
and RPC out of scope because they serve game integrations rather than bots.
Lobby and partner REST APIs are now implemented; the native Social SDK runtime,
its client-side event families, and RPC remain separate platform work.

## On measuring rather than asserting

The same rule applies to speed. `README.md` quotes numbers from
`go test -bench . -benchmem` rather than describing the design as fast, because
the one time it was measured properly it turned out a `bytes.Reader` was being
allocated per frame - 1430 ns and 2 allocations became 1088 ns and 1. That was
invisible from reading the code.

The benchmark that actually settles the design question is
`BenchmarkGuildCreateWrongHandler`: a 158 KB `GUILD_CREATE` arriving at a bot
that has handlers, just not for that event. It costs 431 µs and **one**
allocation, against 1470 µs and **2092** allocations to decode it - so the
handler lookup demonstrably happens before the payload is touched. Asserting
that from the code would have been believable and unfalsifiable; the benchmark
would fail loudly if someone moved the lookup.

Coverage now works the same way: a number produced by a tool that reads the
source, not a number written in a README.
