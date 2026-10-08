package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/razecrs/starlings"
)

func main() {
	bot := starlings.New()
	bot.Slash("ping", "Check the bot is alive", func(i *starlings.InteractionCreate) {
		if err := i.Reply("pong"); err != nil {
			log.Print(err)
		}
	})

	handler, err := bot.InteractionHandler(os.Getenv("DISCORD_PUBLIC_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/interactions", handler)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
