# Dimscord arena adapter

Uses the public `newGuild(JsonNode)` typed constructor. The pinned version's
gateway dispatch procedures are private, so only `guild_create_state` with
`decode_only` coverage is comparable. Other lanes are explicitly unsupported.
