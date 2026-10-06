package starlings

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// StarlogMood selects a pet frame. Build is separate from Info so command and
// compiler output can have its own reaction without pretending to be a warning.
type StarlogMood uint8

const (
	StarlogCalm StarlogMood = iota
	StarlogBuild
	StarlogWarn
	StarlogError
)

// StarlogFrame is one code-native animation frame. Artwork can be replaced
// later without changing the logger or its public API.
type StarlogFrame []string

// StarlogPet is an original terminal-native Star Pet. Every mood is an
// animation, so pets can blink, bounce, jump, celebrate builds, and react to
// warnings. Frames can have any height; Starlog clips them to the dashboard.
type StarlogPet struct {
	Name       string
	Variant    string
	Color      int // ANSI 256-colour foreground; zero uses the dashboard accent.
	FrameDelay time.Duration
	Picture    *StarlogSpriteSheet
	Calm       []StarlogFrame
	Build      []StarlogFrame
	Warn       []StarlogFrame
	Error      []StarlogFrame
}

// StarlogPlayback is one immutable snapshot for the optional now-playing strip.
type StarlogPlayback struct {
	Provider string
	Title    string
	Artist   string
	Position time.Duration
	Duration time.Duration
	Playing  bool
}

// StarlogPlaybackSource supplies live player state. FFmpegOpusProvider
// implements it, and custom players only need this one snapshot method.
type StarlogPlaybackSource interface {
	StarlogPlayback() StarlogPlayback
}

// StarPetVariant selects one of Starlog's built-in colour personalities.
type StarPetVariant uint8

const (
	StarPetNova StarPetVariant = iota
	StarPetComet
	StarPetNebula
)

// NewStarPet returns Starlog's original animated mascot. The shape is kept in
// code for now; the animation contract lets us swap in polished original art
// later without breaking users.
func NewStarPet(variant StarPetVariant) StarlogPet {
	pet := StarlogPet{
		Name:       "Luma",
		Variant:    "nova",
		Color:      220,
		FrameDelay: 160 * time.Millisecond,
		Picture:    newBuiltInSpriteSheet(starPetLumaPNG),
		Calm: []StarlogFrame{
			{`    *`, `  .---.`, ` ( o o )`, `  \_^_/`, `   / \`},
			{`   *`, `  .---.`, ` ( - - )`, `  \_^_/`, `   / \`},
			{`    *`, `  .---.`, ` ( o o )`, `  \_^_/`, `  _/ \_`},
		},
		Build: []StarlogFrame{
			{`    *`, `  .---.  +`, ` ( ^ ^ ) /`, `  \_v_/`, `   / \`},
			{``, `    *  +`, `  .---./`, ` ( ^ ^ )`, `  \_v_/`, `  _/ \_`},
			{`    +`, `  *---*`, ` ( ^ ^ )`, `  \_v_/`, `   / \`},
		},
		Warn: []StarlogFrame{
			{`    *`, `  .---. !`, ` ( o O )`, `  \_~_/`, `   / \`},
			{`   *`, ` !.---.`, ` ( O o )`, `  \_~_/`, `  _/ \_`},
		},
		Error: []StarlogFrame{
			{`    *`, `  .---.`, ` ( x x )`, `  \_-_/`, `  _/ \_`},
			{`   *`, `  .---.`, ` ( > < )`, `  \_-_/`, `   / \`},
		},
	}
	switch variant {
	case StarPetComet:
		pet.Name, pet.Variant, pet.Color = "Comet", "comet", 45
		pet.Picture = newBuiltInSpriteSheet(starPetCometPNG)
	case StarPetNebula:
		pet.Name, pet.Variant, pet.Color = "Nebula", "nebula", 213
		pet.Picture = newBuiltInSpriteSheet(starPetNebulaPNG)
	}
	return pet
}

// StarlogField is one structured slog attribute after formatting.
type StarlogField struct {
	Key   string
	Value string
}

// StarlogEntry is one immutable item in the bounded in-memory log history.
type StarlogEntry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Fields  []StarlogField
	Shard   int
	Mood    StarlogMood
}

// StarlogMetrics is the dashboard's current runtime snapshot.
type StarlogMetrics struct {
	Uptime        time.Duration
	CPUPercent    float64
	HeapBytes     uint64
	Goroutines    int
	LogsPerSecond float64
	TotalLogs     uint64
	DebugLogs     uint64
	InfoLogs      uint64
	WarnLogs      uint64
	ErrorLogs     uint64
	Shards        []ShardStatus
	LogsByShard   map[int]uint64
}

// StarlogOption changes the optional logger without affecting Client options.
type StarlogOption func(*Starlog)

// StarlogTUI controls the fullscreen dashboard. It falls back to streaming
// output when the destination is not an interactive terminal.
func StarlogTUI(enabled bool) StarlogOption { return func(s *Starlog) { s.tui = enabled } }

// StarlogDashboard explicitly selects the full live terminal application.
// It is the default, but naming it can make shared configuration clearer.
func StarlogDashboard() StarlogOption { return StarlogTUI(true) }

// StarlogStreaming selects styled records without the fullscreen application.
func StarlogStreaming() StarlogOption { return StarlogTUI(false) }

// StarlogColor controls ANSI colours. Non-terminal outputs are always plain.
func StarlogColor(enabled bool) StarlogOption { return func(s *Starlog) { s.color = enabled } }

// StarlogWithPet selects one mascot. Pass an empty StarlogPet to disable pets.
func StarlogWithPet(pet StarlogPet) StarlogOption {
	return func(s *Starlog) {
		s.pet = pet
		if pet.Name == "" {
			s.pets = nil
		} else {
			s.pets = []StarlogPet{pet}
		}
	}
}

// StarlogWithPets installs a small dashboard crew. The first pet also supplies
// reaction art for streaming output and styled sinks.
func StarlogWithPets(pets ...StarlogPet) StarlogOption {
	return func(s *Starlog) {
		s.pets = append([]StarlogPet(nil), pets...)
		if len(s.pets) > 0 {
			s.pet = s.pets[0]
		} else {
			s.pet = StarlogPet{}
		}
	}
}

// StarlogNoPets removes mascot art while keeping every dashboard and logging
// feature. It is equivalent to StarlogWithPets() but reads better in config.
func StarlogNoPets() StarlogOption { return StarlogWithPets() }

// StarlogWithPlayback attaches a real player to the optional now-playing strip.
func StarlogWithPlayback(source StarlogPlaybackSource) StarlogOption {
	return func(s *Starlog) { s.FollowPlayback(source) }
}

// StarlogSystemMedia uses the operating system's active media session as a
// fallback when no attached Discord voice/file source is currently active.
// Windows uses System Media Transport Controls and Linux uses MPRIS.
func StarlogSystemMedia() StarlogOption {
	return func(s *Starlog) { s.systemMedia = true }
}

// StarlogStreamArt controls whether warning, error, and build records show a
// compact pet frame when the fullscreen dashboard is disabled.
func StarlogStreamArt(enabled bool) StarlogOption {
	return func(s *Starlog) { s.streamArt = enabled }
}

// StarlogOutput sends terminal output somewhere other than os.Stdout.
func StarlogOutput(w io.Writer) StarlogOption {
	return func(s *Starlog) {
		if w == nil {
			w = io.Discard
		}
		s.output = w
		s.outputFD = -1
		if file, ok := w.(*os.File); ok {
			s.outputFD = int(file.Fd())
		}
	}
}

// StarlogHistory changes the bounded record count shown by Snapshot and TUI.
func StarlogHistory(entries int) StarlogOption {
	return func(s *Starlog) {
		if entries > 0 {
			s.history = entries
		}
	}
}

// StarlogRefresh changes the dashboard redraw period.
func StarlogRefresh(period time.Duration) StarlogOption {
	return func(s *Starlog) {
		if period > 0 {
			s.refresh = period
		}
	}
}

// Starlog is both a structured logger and an optional fullscreen terminal
// dashboard. It is safe for concurrent handlers and subprocess writers.
type Starlog struct {
	name      string
	output    io.Writer
	outputFD  int
	tui       bool
	color     bool
	streamArt bool
	pet       StarlogPet
	pets      []StarlogPet
	history   int
	refresh   time.Duration
	startedAt time.Time
	logger    *slog.Logger

	level slog.LevelVar

	client atomic.Pointer[Client]
	active atomic.Bool
	closed atomic.Bool

	mu          sync.RWMutex
	entries     []StarlogEntry
	entryHead   int
	total       uint64
	levels      [4]uint64
	shardLogs   map[int]uint64
	lastMood    StarlogMood
	lastMoodAt  time.Time
	rateSamples []starlogRateSample
	cpu         starlogCPUSample
	playback    StarlogPlaybackSource
	systemMedia bool
	media       *starlogSystemMedia

	writeMu         sync.Mutex
	metricsMu       sync.Mutex
	metricsAt       time.Time
	runtimeHeap     uint64
	runtimeGo       int
	runtimeTotalCPU float64
	runtimeIdleCPU  float64
	frameMu         sync.Mutex
	lastFrame       []string
	frameWidth      int
	frameHeight     int
	scrollOffset    atomic.Int64
	inputBuffer     []byte
	sinksMu         sync.RWMutex
	sinks           []*StarlogSink
	stop            context.CancelFunc
	restoreTerminal func()
	start           sync.Once
	close           sync.Once
	wg              sync.WaitGroup
}

type starlogRateSample struct {
	at    time.Time
	total uint64
}

type starlogCPUSample struct {
	at, total, idle float64
	percent         float64
}

// NewStarlog creates the optional terminal logger. The TUI, colour, bounded
// history, and animated Star Pet are enabled by default; all have independent
// options.
func NewStarlog(name string, options ...StarlogOption) *Starlog {
	if strings.TrimSpace(name) == "" {
		name = "starlings"
	}
	nova := NewStarPet(StarPetNova)
	s := &Starlog{
		name:      name,
		output:    os.Stdout,
		outputFD:  int(os.Stdout.Fd()),
		tui:       true,
		color:     os.Getenv("NO_COLOR") == "",
		streamArt: true,
		pet:       nova,
		pets: []StarlogPet{
			nova,
			NewStarPet(StarPetComet),
			NewStarPet(StarPetNebula),
		},
		history:   250,
		refresh:   time.Second / 60,
		startedAt: time.Now(),
		shardLogs: make(map[int]uint64),
	}
	for _, option := range options {
		option(s)
	}
	s.level.Set(slog.LevelDebug)
	s.logger = slog.New(&starlogHandler{core: s})
	return s
}

// WithStarlog installs Starlog as the client's logger and attaches live bot
// metrics. RunContext automatically starts and restores the TUI.
func WithStarlog(log *Starlog) Option {
	return func(c *Client) {
		if log == nil {
			return
		}
		c.starlog = log
		c.log = log.Logger()
		log.Attach(c)
	}
}

// Attach supplies bot and shard metrics. Starlog is also useful standalone.
func (s *Starlog) Attach(c *Client) {
	if c != nil {
		s.client.Store(c.rootClient())
	}
}

// FollowPlayback changes the real player shown in the dashboard. Passing nil
// removes the strip. VoiceConnection.Play attaches compatible providers
// automatically when the connection belongs to a client using Starlog.
func (s *Starlog) FollowPlayback(source StarlogPlaybackSource) {
	s.mu.Lock()
	s.playback = source
	s.mu.Unlock()
}

func (s *Starlog) playbackSnapshot() StarlogPlayback {
	s.mu.RLock()
	source := s.playback
	s.mu.RUnlock()
	if source != nil {
		playback := source.StarlogPlayback()
		if playback.Title != "" {
			return playback
		}
	}
	if s.media != nil {
		return s.media.StarlogPlayback()
	}
	return StarlogPlayback{}
}

// Logger returns the standard slog facade used by Starlings and application code.
func (s *Starlog) Logger() *slog.Logger { return s.logger }

// SetLevel changes the minimum retained and displayed slog level.
func (s *Starlog) SetLevel(level slog.Level) { s.level.Set(level) }

// Start activates the fullscreen dashboard when possible. It is safe to call
// even when RunContext will also start it.
func (s *Starlog) Start(ctx context.Context) bool {
	if s.closed.Load() {
		return false
	}
	started := false
	s.start.Do(func() {
		if !s.tui || !s.interactive() {
			return
		}
		ctx, cancel := context.WithCancel(ctx)
		s.stop = cancel
		if s.systemMedia {
			s.media = newStarlogSystemMedia(ctx)
		}
		s.restoreTerminal = prepareStarlogInteraction()
		s.active.Store(true)
		s.writeMu.Lock()
		_, _ = io.WriteString(s.output, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h\x1b[2J\x1b[H")
		s.writeMu.Unlock()
		s.wg.Add(1)
		go s.renderLoop(ctx)
		started = true
	})
	return started || s.active.Load()
}

// Close stops the dashboard and restores the terminal. It never closes the
// configured output writer.
func (s *Starlog) Close() error {
	s.close.Do(func() {
		s.closed.Store(true)
		if s.stop != nil {
			s.stop()
		}
		s.wg.Wait()
		if s.active.Swap(false) {
			s.writeMu.Lock()
			_, _ = io.WriteString(s.output, "\x1b[?1006l\x1b[?1000l\x1b[?25h\x1b[?1049l")
			s.writeMu.Unlock()
		}
		if s.restoreTerminal != nil {
			s.restoreTerminal()
		}
	})
	return nil
}

// Active reports whether Starlog currently owns an interactive terminal.
func (s *Starlog) Active() bool { return s.active.Load() }

// Scroll moves the dashboard log viewport. Positive values move toward older
// output and negative values move back toward the live tail.
func (s *Starlog) Scroll(lines int) {
	for {
		current := s.scrollOffset.Load()
		next := max(int64(0), current+int64(lines))
		if s.scrollOffset.CompareAndSwap(current, next) {
			return
		}
	}
}

// FollowLogs returns the dashboard viewport to the newest output.
func (s *Starlog) FollowLogs() { s.scrollOffset.Store(0) }

// ScrollOffset reports how many rendered rows the viewport is behind live.
func (s *Starlog) ScrollOffset() int { return int(s.scrollOffset.Load()) }

// Snapshot returns deep copies of the newest retained entries.
func (s *Starlog) Snapshot() []StarlogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]StarlogEntry, len(s.entries))
	for i := range s.entries {
		source := i
		if len(s.entries) == s.history {
			source = (s.entryHead + i) % len(s.entries)
		}
		out[i] = s.entries[source]
		out[i].Fields = append([]StarlogField(nil), out[i].Fields...)
	}
	return out
}

// Metrics returns the latest dashboard counters without starting the TUI.
func (s *Starlog) Metrics() StarlogMetrics { return s.metrics(time.Now()) }

// Log writes through the same structured path as slog.
func (s *Starlog) Log(ctx context.Context, level slog.Level, message string, args ...any) {
	s.Logger().Log(ctx, level, message, args...)
}

func (s *Starlog) Debug(message string, args ...any) {
	s.Logger().Debug(message, args...)
}
func (s *Starlog) Info(message string, args ...any) { s.Logger().Info(message, args...) }
func (s *Starlog) Warn(message string, args ...any) { s.Logger().Warn(message, args...) }
func (s *Starlog) Error(message string, args ...any) {
	s.Logger().Error(message, args...)
}

// Build records compiler, downloader, or subprocess progress with the pet's
// build reaction while keeping the semantic log level at Info.
func (s *Starlog) Build(message string, args ...any) {
	args = append(args, "starlog.mood", "build")
	s.Logger().Info(message, args...)
}

// Write implements io.Writer, so cmd.Stdout and cmd.Stderr can point directly
// at Starlog. Each non-empty line becomes an Info record.
func (s *Starlog) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.ReplaceAll(string(p), "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			s.Info(line, "source", "process")
		}
	}
	return len(p), nil
}

// Writer returns a line-buffered subprocess writer with a chosen level and source.
func (s *Starlog) Writer(level slog.Level, source string) io.WriteCloser {
	return &starlogWriter{log: s, level: level, source: source}
}

// BuildWriter is a subprocess writer that keeps the Star Pet in its animated
// build/jump state while output is arriving.
func (s *Starlog) BuildWriter(source string) io.WriteCloser {
	return &starlogWriter{log: s, level: slog.LevelInfo, source: source, mood: StarlogBuild}
}

type starlogWriter struct {
	mu     sync.Mutex
	log    *Starlog
	level  slog.Level
	source string
	mood   StarlogMood
	buf    bytes.Buffer
}

func (w *starlogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	_, _ = w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Put the incomplete tail back for the next write.
			if line != "" {
				var tail bytes.Buffer
				tail.WriteString(line)
				_, _ = tail.ReadFrom(&w.buf)
				w.buf = tail
			}
			break
		}
		line = strings.TrimSpace(line)
		if line != "" {
			w.log.logWriterLine(w.level, w.mood, w.source, line)
		}
	}
	return n, nil
}

func (w *starlogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if line := strings.TrimSpace(w.buf.String()); line != "" {
		w.log.logWriterLine(w.level, w.mood, w.source, line)
	}
	w.buf.Reset()
	return nil
}

func (s *Starlog) logWriterLine(level slog.Level, mood StarlogMood, source, line string) {
	args := []any{"source", source}
	if mood == StarlogBuild {
		args = append(args, "starlog.mood", "build")
	}
	s.Log(context.Background(), level, line, args...)
}

type starlogHandler struct {
	core   *Starlog
	attrs  []slog.Attr
	groups []string
}

func (h *starlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.core.level.Level()
}

func (h *starlogHandler) Handle(_ context.Context, record slog.Record) error {
	entry := StarlogEntry{Time: record.Time, Level: record.Level, Message: sanitizeUntrustedText(record.Message), Shard: -1}
	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	for _, attr := range h.attrs {
		appendStarlogAttr(&entry, h.groups, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendStarlogAttr(&entry, h.groups, attr)
		return true
	})
	if entry.Level >= slog.LevelError {
		entry.Mood = StarlogError
	} else if entry.Level >= slog.LevelWarn {
		entry.Mood = StarlogWarn
	}
	h.core.record(entry)
	return nil
}

func (h *starlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	next.groups = append([]string(nil), h.groups...)
	return &next
}

func (h *starlogHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.attrs = append([]slog.Attr(nil), h.attrs...)
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

func appendStarlogAttr(entry *StarlogEntry, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		next := append(append([]string(nil), groups...), attr.Key)
		for _, child := range attr.Value.Group() {
			appendStarlogAttr(entry, next, child)
		}
		return
	}
	key := sanitizeUntrustedText(strings.Join(append(append([]string(nil), groups...), attr.Key), "."))
	value := sanitizeUntrustedText(starlogValue(attr.Value))
	if key == "starlog.mood" {
		switch value {
		case "build":
			entry.Mood = StarlogBuild
		case "warn":
			entry.Mood = StarlogWarn
		case "error":
			entry.Mood = StarlogError
		}
		return
	}
	if key == "shard" {
		if shard, err := strconv.Atoi(value); err == nil {
			entry.Shard = shard
		}
	}
	entry.Fields = append(entry.Fields, StarlogField{Key: key, Value: value})
}

func starlogValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'f', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(value.Any())
	}
}

func (s *Starlog) record(entry StarlogEntry) {
	s.mu.Lock()
	s.total++
	index := starlogLevelIndex(entry.Level)
	s.levels[index]++
	if entry.Shard >= 0 {
		s.shardLogs[entry.Shard]++
	}
	if entry.Mood != StarlogCalm {
		s.lastMood = entry.Mood
		s.lastMoodAt = entry.Time
	}
	if len(s.entries) == s.history {
		s.entries[s.entryHead] = entry
		s.entryHead = (s.entryHead + 1) % len(s.entries)
	} else {
		s.entries = append(s.entries, entry)
	}
	s.mu.Unlock()
	s.writeSinks(entry)

	if s.active.Load() {
		return
	}
	s.writeStream(entry)
}

func starlogLevelIndex(level slog.Level) int {
	switch {
	case level >= slog.LevelError:
		return 3
	case level >= slog.LevelWarn:
		return 2
	case level >= slog.LevelInfo:
		return 1
	default:
		return 0
	}
}

func (s *Starlog) interactive() bool {
	return s.outputFD >= 0 && term.IsTerminal(s.outputFD) && prepareStarlogTerminal(s.outputFD)
}
