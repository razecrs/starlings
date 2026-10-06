# discord.js arena adapter

Uses the pinned checkout's `Client`, `GUILD_CREATE` websocket handler, `MessageCreateAction`, collection-backed caches, and `GuildMember.permissionsIn`. Repeated message fixtures are removed from the message cache immediately before dispatch because replaying one gateway sequence otherwise exercises Discord's duplicate-object shortcut.

All canonical workloads are supported. `malformed_frame` enters through the same envelope parser used before dispatch. No login or network API is used.
