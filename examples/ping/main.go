// Command ping is the smallest Starlings bot: a /ping slash command.
//
// Put the bot token in DISCORD_TOKEN, or in a .env file, and run it:
//
//	go run ./examples/ping
//
// Set DISCORD_GUILD_ID to a server you are testing in so the command appears
// there at once.
package main

import (
	"log"

	"github.com/razecrs/starlings"
)

func main() {
	bot := starlings.New()
	bot.Slash("ping", "Is the bot alive?", ping)
	log.Fatal(bot.Run())
}

func ping() string {
	return "Pong!"
}
