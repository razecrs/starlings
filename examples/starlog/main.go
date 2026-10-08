package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"

	"github.com/razecrs/starlings"
	_ "github.com/razecrs/starlings/starpets"
	"github.com/razecrs/starlings/sysmedia"
)

func main() {
	logs := starlings.NewStarlog("my bot",
		starlings.StarlogColor(true),
		sysmedia.Option(),
		starlings.StarlogWithPets(
			starlings.NewStarPet(starlings.StarPetNova),
			starlings.NewStarPet(starlings.StarPetComet),
			starlings.NewStarPet(starlings.StarPetNebula),
		))
	bot := starlings.New(os.Getenv("DISCORD_TOKEN"),
		starlings.WithStarlog(logs))

	bot.On(func(ready *starlings.Ready) {
		logs.Info("ready", "user", ready.User.Tag())
	})

	bot.Command("build", func(message *starlings.MessageCreate, _ []string) {
		logs.Build("running go test")
		cmd := exec.CommandContext(context.Background(), "go", "test", "./...")
		stdout := logs.BuildWriter("go test")
		stderr := logs.Writer(slog.LevelWarn, "go test")
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			_ = stdout.Close()
			_ = stderr.Close()
			logs.Error("build failed", "err", err)
			return
		}
		_ = stdout.Close()
		_ = stderr.Close()
		logs.Info("build passed")
	})

	if err := bot.Run(); err != nil {
		logs.Error("bot stopped", "err", err)
	}
}
