package starlings

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// StarlogColorPolicy controls whether a styled sink emits ANSI colour.
// Auto keeps ordinary files escape-free, Always is useful for ANSI-aware log
// viewers, and Never retains Starlog's Unicode layout without colour codes.
type StarlogColorPolicy uint8

const (
	StarlogColorAuto StarlogColorPolicy = iota
	StarlogColorAlways
	StarlogColorNever
)

// StarlogSinkOption customises one independent styled output.
type StarlogSinkOption func(*StarlogSink)

// StarlogSink writes a filtered, styled copy of Starlog records. A sink never
// owns or closes its output, so the caller remains in control of files and
// pipes. It is safe to use from concurrent event handlers.
type StarlogSink struct {
	output io.Writer
	fd     int

	minLevel slog.Level
	maxLevel slog.Level
	color    int
	policy   StarlogColorPolicy
	art      bool
	pet      StarlogPet

	mu sync.Mutex
}

// NewStarlogSink creates a reusable styled destination. By default it accepts
// every level, uses cyan, includes reaction art for non-calm records, and only
// emits ANSI when output is an interactive terminal.
func NewStarlogSink(output io.Writer, options ...StarlogSinkOption) *StarlogSink {
	if output == nil {
		output = io.Discard
	}
	sink := &StarlogSink{
		output:   output,
		fd:       -1,
		minLevel: slog.LevelDebug,
		maxLevel: slog.Level(127),
		color:    81,
		policy:   StarlogColorAuto,
		art:      true,
		pet:      NewStarPet(StarPetNova),
	}
	if file, ok := output.(*os.File); ok {
		sink.fd = int(file.Fd())
	}
	for _, option := range options {
		option(sink)
	}
	return sink
}

// StarlogSinkLevels limits a sink to an inclusive level range.
func StarlogSinkLevels(minimum, maximum slog.Level) StarlogSinkOption {
	return func(s *StarlogSink) { s.minLevel, s.maxLevel = minimum, maximum }
}

// StarlogSinkMinLevel accepts a level and everything more severe.
func StarlogSinkMinLevel(level slog.Level) StarlogSinkOption {
	return func(s *StarlogSink) { s.minLevel = level }
}

// StarlogSinkColor selects an ANSI 256-colour foreground.
func StarlogSinkColor(color int) StarlogSinkOption {
	return func(s *StarlogSink) {
		if color >= 0 && color <= 255 {
			s.color = color
		}
	}
}

// StarlogSinkColorPolicy selects automatic, forced, or disabled ANSI colour.
func StarlogSinkColorPolicy(policy StarlogColorPolicy) StarlogSinkOption {
	return func(s *StarlogSink) { s.policy = policy }
}

// StarlogSinkArt controls the compact Star Pet reaction beside styled records.
func StarlogSinkArt(enabled bool) StarlogSinkOption {
	return func(s *StarlogSink) { s.art = enabled }
}

// StarlogSinkPet changes the sink's pet independently from the full dashboard.
func StarlogSinkPet(pet StarlogPet) StarlogSinkOption {
	return func(s *StarlogSink) { s.pet = pet }
}

// AddSink attaches an independent styled output and returns a removal function.
// Removing a sink never closes its writer.
func (s *Starlog) AddSink(sink *StarlogSink) (remove func()) {
	if sink == nil {
		return func() {}
	}
	s.sinksMu.Lock()
	s.sinks = append(s.sinks, sink)
	s.sinksMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.sinksMu.Lock()
			defer s.sinksMu.Unlock()
			for i, candidate := range s.sinks {
				if candidate == sink {
					s.sinks = append(s.sinks[:i], s.sinks[i+1:]...)
					return
				}
			}
		})
	}
}

// Errors is the short path for a red, illustrated error-only destination.
func (s *Starlog) Errors(output io.Writer, options ...StarlogSinkOption) *StarlogSink {
	base := []StarlogSinkOption{
		StarlogSinkLevels(slog.LevelError, slog.Level(127)),
		StarlogSinkColor(203),
	}
	sink := NewStarlogSink(output, append(base, options...)...)
	s.AddSink(sink)
	return sink
}

// Warnings is the short path for a gold, illustrated warning-and-error
// destination.
func (s *Starlog) Warnings(output io.Writer, options ...StarlogSinkOption) *StarlogSink {
	base := []StarlogSinkOption{
		StarlogSinkMinLevel(slog.LevelWarn),
		StarlogSinkColor(220),
	}
	sink := NewStarlogSink(output, append(base, options...)...)
	s.AddSink(sink)
	return sink
}

// Styled is the short path for an all-level, single-colour destination.
func (s *Starlog) Styled(output io.Writer, color int, options ...StarlogSinkOption) *StarlogSink {
	base := []StarlogSinkOption{StarlogSinkColor(color)}
	sink := NewStarlogSink(output, append(base, options...)...)
	s.AddSink(sink)
	return sink
}

func (s *Starlog) writeSinks(entry StarlogEntry) {
	s.sinksMu.RLock()
	sinks := append([]*StarlogSink(nil), s.sinks...)
	s.sinksMu.RUnlock()
	for _, sink := range sinks {
		sink.write(entry)
	}
}

func (s *StarlogSink) write(entry StarlogEntry) {
	if entry.Level < s.minLevel || entry.Level > s.maxLevel {
		return
	}
	colour := s.useColor()
	code := fmt.Sprintf("\x1b[38;5;%dm", s.color)
	paint := func(value string) string {
		if !colour || value == "" {
			return value
		}
		return code + value + ansiReset
	}

	level := strings.ToUpper(entry.Level.String())
	header := fmt.Sprintf("✦ %-5s  %s", level, entry.Time.Format("2006-01-02 15:04:05"))
	message := entry.Message
	if entry.Shard >= 0 {
		message = fmt.Sprintf("[shard %d] %s", entry.Shard, message)
	}
	for _, field := range entry.Fields {
		message += "  " + field.Key + "=" + field.Value
	}

	lines := []string{paint("╭─ " + header), "│  " + message}
	if s.art && entry.Mood != StarlogCalm && s.pet.Name != "" {
		frame, pictured := StarlogFrame(nil), false
		if colour {
			frame, pictured = renderStarPetPicture(s.pet, entry.Mood, entry.Time, 18)
		}
		if !pictured {
			frame = sinkPetFrame(s.pet, entry.Mood, entry.Time)
		}
		middle := len(frame) / 2
		for i, art := range frame {
			if !pictured {
				art = paint(art)
			}
			line := "│  " + art
			if i == middle {
				line += "  " + message
				lines[1] = ""
			}
			lines = append(lines, line)
		}
	}
	if lines[1] == "" {
		lines = append(lines[:1], lines[2:]...)
	}
	lines = append(lines, paint("╰"+strings.Repeat("─", 34)))

	s.mu.Lock()
	_, _ = fmt.Fprintln(s.output, strings.Join(lines, "\n"))
	s.mu.Unlock()
}

func (s *StarlogSink) useColor() bool {
	switch s.policy {
	case StarlogColorAlways:
		return true
	case StarlogColorNever:
		return false
	default:
		return s.fd >= 0 && term.IsTerminal(s.fd) && prepareStarlogTerminal(s.fd) && os.Getenv("NO_COLOR") == ""
	}
}

func sinkPetFrame(pet StarlogPet, mood StarlogMood, now time.Time) StarlogFrame {
	var animation []StarlogFrame
	switch mood {
	case StarlogBuild:
		animation = pet.Build
	case StarlogWarn:
		animation = pet.Warn
	case StarlogError:
		animation = pet.Error
	default:
		animation = pet.Calm
	}
	if len(animation) == 0 {
		return nil
	}
	delay := pet.FrameDelay
	if delay <= 0 {
		delay = 300 * time.Millisecond
	}
	return animation[int(now.UnixNano()/int64(delay))%len(animation)]
}
