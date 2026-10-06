package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/razecrs/starlings"
)

type demoPlayback struct{ started time.Time }

func (d demoPlayback) StarlogPlayback() starlings.StarlogPlayback {
	duration := 3*time.Minute + 42*time.Second
	position := time.Since(d.started) % duration
	return starlings.StarlogPlayback{
		Provider: "Spotify", Title: "Starlight Runtime", Artist: "The Star Pets",
		Position: position, Duration: duration, Playing: true,
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	logs := starlings.NewStarlog("grand publish preview",
		starlings.StarlogColor(true),
		starlings.StarlogHistory(500),
		starlings.StarlogWithPlayback(demoPlayback{started: time.Now()}),
		starlings.StarlogWithPets(
			starlings.NewStarPet(starlings.StarPetNova),
			starlings.NewStarPet(starlings.StarPetComet),
			starlings.NewStarPet(starlings.StarPetNebula),
		))
	if !logs.Start(ctx) {
		fmt.Fprintln(os.Stderr, "starlogdemo needs an interactive terminal")
		return
	}
	defer logs.Close()

	for index := range 45 {
		logs.Info("gateway event handled cleanly", "shard", index%3, "event", "MESSAGE_CREATE", "sequence", 9100+index)
	}
	logs.Warn("this intentionally long warning demonstrates that Starlog now wraps the complete response instead of truncating the useful details at the right edge",
		"shard", 1, "retry", 850*time.Millisecond)
	logs.Error("example recovery event", "component", "voice", "recovered", true)
	logs.Build("release checks are running", "target", "windows/linux")

	ticker := time.NewTicker(850 * time.Millisecond)
	defer ticker.Stop()
	step := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			step++
			level := slog.LevelInfo
			if step%13 == 0 {
				level = slog.LevelWarn
			}
			logs.Log(ctx, level, "live shard heartbeat", "shard", step%3, "ping", time.Duration(80+step%45)*time.Millisecond)
		}
	}
}
