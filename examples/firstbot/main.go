// Command firstbot is the smallest useful Starlings bot.
//
// Run it:
//
//	set DISCORD_TOKEN=your-token-here     (Windows)
//	export DISCORD_TOKEN=your-token-here  (macOS/Linux)
//	go run ./examples/firstbot
//
// Then type `!ping` in any channel the bot can see.
package main

import (
	"log"
	"os"
	"strings"

	"github.com/razecrs/starlings"
)

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("set DISCORD_TOKEN first - get one from discord.com/developers/applications")
	}

	// NewCommandBot selects the intents prefix commands need. MessageContent
	// must still be enabled in the developer portal.
	bot := starlings.NewCommandBot(token,
		starlings.WithStatus(starlings.StatusOnline, starlings.Playing("with starlings")),
	)

	bot.Command("ping", func(m *starlings.MessageCreate, args []string) {
		if _, err := m.Reply("pong"); err != nil {
			log.Printf("could not reply: %v", err)
		}
	})

	bot.Command("say", func(m *starlings.MessageCreate, args []string) {
		if len(args) == 0 {
			m.Reply("say what? try `!say hello`")
			return
		}
		// Anything built from user input should suppress mentions, or someone
		// will make the bot ping @everyone for them.
		m.ReplyComplex(starlings.SendData{
			Content:         strings.Join(args, " "),
			AllowedMentions: starlings.NoMentions(),
		})
	})

	bot.On(func(r *starlings.Ready) {
		log.Printf("online as %s", r.User.Tag())
	})

	if err := bot.Run(); err != nil {
		log.Fatal(err)
	}
}
