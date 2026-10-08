# Starlog

Starlog can be a fullscreen terminal dashboard, a styled streaming logger, a standard `slog.Logger`, or any combination of those outputs.

## Dashboard

```go
logs := starlings.NewStarlog("my bot",
	starlings.StarlogDashboard(),
	sysmedia.Option(),
	starlings.StarlogWithPets(
		starlings.NewStarPet(starlings.StarPetNova),
		starlings.NewStarPet(starlings.StarPetComet),
	))

bot := starlings.New(token, starlings.WithStarlog(logs))
```

The media strip and the pixel-art pets come from two optional packages, so
bots that do not use them do not link WinRT, D-Bus, or about 5 MB of images:

```go
import (
	"github.com/razecrs/starlings/sysmedia"
	_ "github.com/razecrs/starlings/starpets"
)
```

Without `starpets`, pets draw with terminal characters.

`WithStarlog` attaches shard status and starts/stops the display with `RunContext`. If Starlog is used without a client, call `Start` and `Close` yourself.

The dashboard shows process metrics, shard state, heartbeat latency, sequence numbers, log rate, scrollback, pets, and optional media. Arrow keys or the mouse wheel scroll one line, Page Up/Page Down scroll a page, Home jumps to the oldest retained record, and End resumes live following.

When output is not an interactive terminal, dashboard mode falls back to streaming records. It never writes cursor-control sequences into CI or an ordinary file.

## Pick only the pieces you want

```go
logs := starlings.NewStarlog("worker",
	starlings.StarlogStreaming(),
	starlings.StarlogColor(true),
	starlings.StarlogNoPets(),
	starlings.StarlogHistory(2_000),
	starlings.StarlogRefresh(time.Second/30),
	starlings.StarlogOutput(os.Stderr))
```

- `StarlogDashboard` and `StarlogStreaming` choose fullscreen or records.
- `StarlogWithPet`, `StarlogWithPets`, and `StarlogNoPets` choose the mascot layout.
- `sysmedia.Option` reads Windows media sessions or Linux MPRIS when voice is idle. `StarlogMediaSource` accepts any other `StarlogMediaReader`.
- `StarlogWithPlayback` follows any `StarlogPlaybackSource`; attached FFmpeg voice playback is detected automatically.
- `StarlogStreamArt` controls compact reaction art in streaming mode.
- `StarlogHistory` is a bounded record count, not an unbounded log buffer.

Run `go run ./examples/starlogdemo` for a network-free preview.

## Structured logging

```go
logs.Info("ready", "guilds", count)
logs.Warn("heartbeat late", "shard", shardID, "latency", latency)
logs.Error("request failed", "err", err)

logger := logs.Logger()
logger.InfoContext(ctx, "sync complete", "commands", n)
```

`Logger` is a normal `*slog.Logger`, so packages such as StarDB can share it. `SetLevel` changes the live minimum level.

## Files and filtered sinks

```go
errorsFile, err := os.OpenFile("errors.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
if err != nil {
	log.Fatal(err)
}
defer errorsFile.Close()

logs.Errors(errorsFile,
	starlings.StarlogSinkArt(true),
	starlings.StarlogSinkColorPolicy(starlings.StarlogColorNever))
logs.Warnings(os.Stderr)
logs.Styled(os.Stdout, 45)
```

Regular files default to plain Unicode without ANSI escapes. Use `StarlogColorAlways` only when the destination understands terminal colour. `AddSink` returns a removal function for temporary routing.

## Subprocess output

```go
stdout := logs.BuildWriter("go test")
stderr := logs.Writer(slog.LevelWarn, "go test")
defer stdout.Close()
defer stderr.Close()

cmd.Stdout = stdout
cmd.Stderr = stderr
err := cmd.Run()
```

Each writer buffers partial lines and flushes the last one on `Close`. Closing it matters when a subprocess exits without a final newline.

## Deployment notes

- Use streaming mode for services where another supervisor owns the terminal.
- Disable colour for JSON/text collectors unless their viewer understands ANSI.
- Keep file permissions restrictive; Starlog does not rotate files for you.
- The dashboard redraw rate affects presentation, not gateway or heartbeat timing.
- System-media integration is optional. Failure to find a desktop player leaves the card empty and does not stop the logger.
