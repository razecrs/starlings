// Command ping is a minimal starlings bot: it replies "pong" to "ping".
//
// Run it with a bot token in the environment:
//
//	DISCORD_TOKEN=... go run ./examples/ping
package main

import (
	"github.com/razecrs/starlings"
	"log"
	"os"
)

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("set DISCORD_TOKEN")
	}

	// MessageContent is a privileged intent: switch it on for the bot in the
	// Discord developer portal, or m.Content arrives empty.
	bot := starlings.New(token,
		starlings.WithIntents(starlings.IntentGuilds|starlings.IntentGuildMessages|starlings.IntentMessageContent),
		starlings.WithStatus(starlings.StatusOnline, starlings.Playing("with starlings")),
	)

	bot.On(func(r *starlings.Ready) {
		log.Printf("online as %s", r.User.Tag())
	})

	bot.On(func(m *starlings.MessageCreate) {
		// Without this a bot that echoes anything will talk to itself forever.
		if m.IsFromBot() {
			return
		}
		if m.Content == "ping" {
			if _, err := m.Reply("pong"); err != nil {
				log.Printf("replying: %v", err)
			}
		}
	})

	// Run blocks until Ctrl-C, reconnecting on its own if the connection drops.
	if err := bot.Run(); err != nil {
		log.Fatal(err)
	}
}
