# discord.py arena adapter

Uses `ConnectionState.parsers` for gateway dispatch, `Guild`'s dictionary-backed `get_member`, and `GuildChannel.permissions_for`. The synchronous dispatch callback models the typed event consumer without scheduling coroutine overhead. The independent message fixture has no populated matching guild, so discord.py correctly creates its partial channel model.

All canonical workloads are supported. No client login or network request is made.
