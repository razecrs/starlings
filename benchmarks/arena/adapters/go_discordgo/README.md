# DiscordGo arena adapter

DiscordGo does not expose its offline websocket frame dispatcher. This adapter
uses its public gateway models and calls the exported `State.OnInterface`
consumer that DiscordGo's private dispatcher calls. It also reproduces
DiscordGo's internal guild-ID propagation for nested objects. Typed callback
counting is adapter-owned and disclosed in `info`; state work is real.

No network connection is opened and no competitor source is changed.
