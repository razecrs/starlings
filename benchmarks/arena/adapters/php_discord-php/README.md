# DiscordPHP arena adapter

Loads the pinned Composer graph without constructing `Discord`, because its
constructor calls `connectWs()`. The raw frame method is protected and handlers
depend on that connecting client graph. All warm lanes explicitly exit 2; cold
start measures safe library/autoloader readiness only.
