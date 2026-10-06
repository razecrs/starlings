package starlings

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestStarlogStreamsStructuredPlainText(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("test",
		StarlogTUI(false),
		StarlogColor(false),
		StarlogWithPet(StarlogPet{}),
		StarlogOutput(&output),
		StarlogHistory(8),
	)
	logger := log.Logger().With("component", "gateway").WithGroup("discord")
	logger.Info("connected", "shard", 2, "latency", 12*time.Millisecond)

	got := output.String()
	for _, want := range []string{"INFO", "connected", "component=gateway", "discord.shard=2", "discord.latency=12ms"} {
		if !strings.Contains(got, want) {
			t.Errorf("stream output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("plain stream contains ANSI escapes: %q", got)
	}
}

func TestStarlogHistoryMoodsAndMetrics(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("test",
		StarlogTUI(false), StarlogColor(false), StarlogStreamArt(false),
		StarlogOutput(&output), StarlogHistory(2),
	)
	log.Info("one")
	log.Build("two")
	log.Error("three")

	entries := log.Snapshot()
	if len(entries) != 2 || entries[0].Message != "two" || entries[1].Message != "three" {
		t.Fatalf("bounded history = %#v", entries)
	}
	if entries[0].Mood != StarlogBuild || entries[1].Mood != StarlogError {
		t.Fatalf("moods = %v, %v", entries[0].Mood, entries[1].Mood)
	}
	stats := log.Metrics()
	if stats.TotalLogs != 3 || stats.InfoLogs != 2 || stats.ErrorLogs != 1 {
		t.Fatalf("metrics = %#v", stats)
	}
}

func TestStarlogSubprocessWriter(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("test",
		StarlogTUI(false), StarlogColor(false), StarlogWithPet(StarlogPet{}),
		StarlogOutput(&output),
	)
	w := log.Writer(slog.LevelWarn, "ffmpeg")
	_, _ = w.Write([]byte("frame 1"))
	if len(log.Snapshot()) != 0 {
		t.Fatal("partial command line was emitted early")
	}
	_, _ = w.Write([]byte(" done\nnext\n"))
	_ = w.Close()
	entries := log.Snapshot()
	if len(entries) != 2 || entries[0].Message != "frame 1 done" || entries[1].Message != "next" {
		t.Fatalf("command entries = %#v", entries)
	}
	if entries[0].Level != slog.LevelWarn || !strings.Contains(output.String(), "source=ffmpeg") {
		t.Fatalf("command output = %q", output.String())
	}
}

func TestStarlogWriterCloseFlushesTail(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("test", StarlogTUI(false), StarlogColor(false),
		StarlogWithPet(StarlogPet{}), StarlogOutput(&output))
	w := log.Writer(slog.LevelInfo, "tool")
	_, _ = w.Write([]byte("no final newline"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	entries := log.Snapshot()
	if len(entries) != 1 || entries[0].Message != "no final newline" {
		t.Fatalf("flushed entries = %#v", entries)
	}
}

func TestStarlogBuildWriterDrivesPetMood(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("test", StarlogTUI(false), StarlogColor(false),
		StarlogStreamArt(false), StarlogOutput(&output))
	w := log.BuildWriter("compiler")
	_, _ = w.Write([]byte("building package\n"))
	_ = w.Close()
	entries := log.Snapshot()
	if len(entries) != 1 || entries[0].Mood != StarlogBuild {
		t.Fatalf("build writer entries = %#v", entries)
	}
}

func TestStarPetAnimationAndVariants(t *testing.T) {
	nova := NewStarPet(StarPetNova)
	comet := NewStarPet(StarPetComet)
	nebula := NewStarPet(StarPetNebula)
	if nova.Name == "" || nova.Variant == comet.Variant || comet.Variant == nebula.Variant {
		t.Fatalf("variants = %q, %q, %q", nova.Variant, comet.Variant, nebula.Variant)
	}
	if nova.Name == comet.Name || comet.Name == nebula.Name || nova.Name == nebula.Name {
		t.Fatalf("pet names are not distinct: %q, %q, %q", nova.Name, comet.Name, nebula.Name)
	}
	if bytes.Equal(nova.Picture.PNG, comet.Picture.PNG) || bytes.Equal(comet.Picture.PNG, nebula.Picture.PNG) || bytes.Equal(nova.Picture.PNG, nebula.Picture.PNG) {
		t.Fatal("built-in pets share a sprite sheet")
	}
	log := NewStarlog("pet", StarlogWithPet(nova), StarlogTUI(false))
	first := strings.Join(log.petFrame(StarlogBuild, time.Unix(0, 0)), "\n")
	second := strings.Join(log.petFrame(StarlogBuild, time.Unix(0, int64(nova.FrameDelay))), "\n")
	if first == second {
		t.Fatal("build animation did not advance to its jump frame")
	}
}

func TestStarlogSpotifyCardUsesProviderTheme(t *testing.T) {
	log := NewStarlog("media", StarlogColor(false))
	card := strings.Join(log.musicCard(StarlogPlayback{
		Provider: "Spotify", Title: "Human Music", Artist: "A Person",
		Position: time.Minute, Duration: 3 * time.Minute, Playing: true,
	}, time.UnixMilli(160), 100), "\n")
	for _, want := range []string{"SPOTIFY", "Human Music", "A Person", "PLAYING", "01:00 / 03:00"} {
		if !strings.Contains(card, want) {
			t.Errorf("media card %q does not contain %q", card, want)
		}
	}
}

type fixedPlayback StarlogPlayback

func (f fixedPlayback) StarlogPlayback() StarlogPlayback { return StarlogPlayback(f) }

func TestStarlogExplicitPlaybackPrecedesSystemFallback(t *testing.T) {
	log := NewStarlog("media", StarlogWithPlayback(fixedPlayback{Title: "voice"}))
	log.media = &starlogSystemMedia{}
	log.media.snapshot.Store(&starlogMediaSnapshot{playback: StarlogPlayback{Title: "spotify"}, at: time.Now()})
	if got := log.playbackSnapshot().Title; got != "voice" {
		t.Fatalf("playback title = %q, want voice", got)
	}
	log.FollowPlayback(fixedPlayback{})
	if got := log.playbackSnapshot().Title; got != "spotify" {
		t.Fatalf("fallback title = %q, want spotify", got)
	}
}

func TestStarlogNonTerminalFallbackAndShardStatus(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("test", StarlogOutput(&output))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if log.Start(ctx) || log.Active() {
		t.Fatal("buffer output should not activate the fullscreen TUI")
	}

	c := New("token", WithStarlog(log))
	c.gw.connected.Store(true)
	c.gw.latency.Store(int64(42 * time.Millisecond))
	c.seq.Store(9)
	session := "resume-me"
	c.sessionID.Store(&session)
	status := c.ShardStatuses()
	if len(status) != 1 || !status[0].Connected || status[0].Latency != 42*time.Millisecond || status[0].Sequence != 9 || !status[0].Resumable {
		t.Fatalf("shard status = %#v", status)
	}
	c.gatewayLogger().Info("shard log")
	if got := log.Metrics().LogsByShard[0]; got != 1 {
		t.Fatalf("shard log count = %d", got)
	}
}

func TestStarlogDashboardRendering(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("dashboard", StarlogOutput(&output), StarlogColor(false))
	log.Info("hello tui")
	log.render(time.Now())
	got := output.String()
	for _, want := range []string{"STARLOG / dashboard", "CPU", "PET DECK", "SHARD MATRIX", "hello tui"} {
		if !strings.Contains(got, want) {
			t.Errorf("dashboard does not contain %q", want)
		}
	}
	frame := got
	if marker := strings.LastIndex(frame, "\x1b[2J\x1b[H"); marker >= 0 {
		frame = frame[marker+len("\x1b[2J\x1b[H"):]
	}
	plain := stripANSI(frame)
	for row, line := range strings.Split(plain, "\n") {
		if width := visibleWidth(line); width != 100 {
			t.Errorf("dashboard row %d width = %d, want 100: %q", row, width, line)
		}
	}
}

func TestStarlogDashboardWrapsAndScrollsWithoutLosingColour(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("scroll", StarlogOutput(&output), StarlogColor(true), StarlogNoPets())
	long := strings.Repeat("human-readable-output-", 8) + "TAIL-MARKER"
	log.Warn(long)
	wrapped := wrapVisible(log.formatEntry(log.Snapshot()[0], true), 48)
	if len(wrapped) < 2 || !strings.Contains(strings.Join(wrapped, ""), "TAIL-MARKER") {
		t.Fatalf("long warning was not wrapped losslessly: %#v", wrapped)
	}
	for index, line := range wrapped {
		if visibleWidth(line) > 48 {
			t.Fatalf("wrapped line %d width = %d", index, visibleWidth(line))
		}
	}
	if !strings.Contains(strings.Join(wrapped, ""), ansiWarnBadge) {
		t.Fatal("wrapping stripped the warning colour")
	}

	log.Scroll(7)
	if got := log.ScrollOffset(); got != 7 {
		t.Fatalf("scroll offset = %d, want 7", got)
	}
	log.FollowLogs()
	if got := log.ScrollOffset(); got != 0 {
		t.Fatalf("follow offset = %d, want 0", got)
	}
}

func TestStarlogDashboardInputControlsViewport(t *testing.T) {
	log := NewStarlog("keys", StarlogOutput(io.Discard))
	for _, test := range []struct {
		sequence string
		want     int
	}{
		{"\x1b[A", 1},
		{"\x1b[5~", 11},
		{"\x1b[B", 10},
		{"\x1b[6~", 0},
		{"\x1b[<64;5;5M", 3},
		{"\x1b[<65;5;5M", 0},
	} {
		used, complete := log.readEscapeInput([]byte(test.sequence), 10)
		if !complete || used != len(test.sequence) {
			t.Fatalf("input %q consumed=%d complete=%v", test.sequence, used, complete)
		}
		if got := log.ScrollOffset(); got != test.want {
			t.Fatalf("input %q offset=%d want=%d", test.sequence, got, test.want)
		}
	}
}

func TestStarlogStyledSinksFilterAndKeepFilesClean(t *testing.T) {
	var errorsFile, warningsFile bytes.Buffer
	log := NewStarlog("sinks", StarlogStreaming(), StarlogOutput(io.Discard))
	log.Errors(&errorsFile)
	log.Warnings(&warningsFile)

	log.Info("ordinary")
	log.Warn("careful", "attempt", 2)
	log.Error("broken", "code", 42)

	if strings.Contains(errorsFile.String(), "ordinary") || strings.Contains(errorsFile.String(), "careful") {
		t.Fatalf("error sink accepted lower levels:\n%s", errorsFile.String())
	}
	if !strings.Contains(errorsFile.String(), "broken") || !strings.Contains(errorsFile.String(), "code=42") {
		t.Fatalf("error sink lost its structured record:\n%s", errorsFile.String())
	}
	if !strings.Contains(errorsFile.String(), "╭─ ✦ ERROR") || !strings.Contains(errorsFile.String(), ".---.") {
		t.Fatalf("error sink did not retain layout and reaction art:\n%s", errorsFile.String())
	}
	if strings.Contains(errorsFile.String(), "\x1b[") {
		t.Fatalf("automatic file output contains ANSI escapes: %q", errorsFile.String())
	}
	if strings.Contains(warningsFile.String(), "ordinary") ||
		!strings.Contains(warningsFile.String(), "careful") ||
		!strings.Contains(warningsFile.String(), "broken") {
		t.Fatalf("warning sink level filter is wrong:\n%s", warningsFile.String())
	}
}

func TestStarlogSinkCanForceOneColourAndBeRemoved(t *testing.T) {
	var output bytes.Buffer
	log := NewStarlog("colour", StarlogStreaming(), StarlogOutput(io.Discard))
	sink := NewStarlogSink(&output,
		StarlogSinkColor(196),
		StarlogSinkColorPolicy(StarlogColorAlways),
		StarlogSinkArt(false))
	remove := log.AddSink(sink)

	log.Info("painted")
	remove()
	log.Info("removed")

	got := output.String()
	if !strings.Contains(got, "\x1b[38;5;196m") || !strings.Contains(got, "painted") {
		t.Fatalf("forced single-colour output missing: %q", got)
	}
	if strings.Contains(got, "removed") {
		t.Fatalf("removed sink still received records: %q", got)
	}
}

func TestStarPetPictureRendersTrueColourFrames(t *testing.T) {
	for _, variant := range []StarPetVariant{StarPetNova, StarPetComet, StarPetNebula} {
		pet := NewStarPet(variant)
		frame, ok := renderStarPetPicture(pet, StarlogBuild, time.Unix(0, 0), 22)
		if !ok || len(frame) < 4 {
			t.Fatalf("pictured pet %q did not render: ok=%v rows=%d", pet.Name, ok, len(frame))
		}
		joined := strings.Join(frame, "\n")
		if !strings.Contains(joined, "\x1b[38;2;") {
			t.Fatalf("pictured pet %q is missing true-colour terminal cells", pet.Name)
		}
		if !regexp.MustCompile(`\x1b\[38;2;(?:[1-9][0-9]*);(?:[1-9][0-9]*);(?:[1-9][0-9]*)m`).MatchString(joined) {
			t.Fatalf("pictured pet %q rendered as an all-black silhouette", pet.Name)
		}
		for row, line := range frame {
			if width := visibleWidth(line); width != 22 {
				t.Fatalf("picture %q row %d has width %d, want 22", pet.Name, row, width)
			}
		}
	}
}
