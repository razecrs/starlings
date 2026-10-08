// Command slashbot demonstrates the short API without a resource cache.
// Set DISCORD_TOKEN and DISCORD_GUILD_ID before running it. Never commit tokens.
package main

import (
	"log"

	"github.com/razecrs/starlings"
)

type SayArgs struct {
	Text string `desc:"What to say" min:"1" max:"2000"`
}

func main() {
	bot := starlings.New(starlings.WithStateCache(starlings.MinimalStateConfig()))
	bot.Slash("ping", "Check the bot", func() string { return "pong" })
	bot.Slash("say", "Reply privately", func(a SayArgs) starlings.Response {
		return starlings.Ephemeral(a.Text)
	})
	if err := bot.Run(); err != nil {
		log.Fatal(err)
	}
}
