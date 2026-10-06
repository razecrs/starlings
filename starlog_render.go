package starlings

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"runtime/metrics"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiRed    = "\x1b[38;5;203m"
	ansiGold   = "\x1b[38;5;220m"
	ansiBlue   = "\x1b[38;5;81m"
	ansiGreen  = "\x1b[38;5;84m"
	ansiMuted  = "\x1b[38;5;245m"
	ansiWhite  = "\x1b[38;5;255m"
	ansiPink   = "\x1b[38;5;213m"
	ansiViolet = "\x1b[38;5;141m"
	ansiOrange = "\x1b[38;5;208m"
	ansiFrost  = "\x1b[38;5;159m"
	ansiInk    = "\x1b[38;5;16m"

	ansiWarnBadge  = "\x1b[1;38;5;16;48;5;220m"
	ansiErrorBadge = "\x1b[1;38;5;255;48;5;196m"
)

func (s *Starlog) renderLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.refresh)
	defer ticker.Stop()
	s.render(time.Now())
	for {
		s.readInput()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.render(time.Now())
		}
	}
}

func (s *Starlog) readInput() {
	if s.outputFD < 0 {
		return
	}
	var incoming [256]byte
	for {
		n := readStarlogInput(int(os.Stdin.Fd()), incoming[:])
		if n == 0 {
			break
		}
		s.inputBuffer = append(s.inputBuffer, incoming[:n]...)
		if n < len(incoming) {
			break
		}
	}
	if len(s.inputBuffer) > 4096 {
		s.inputBuffer = s.inputBuffer[len(s.inputBuffer)-4096:]
	}
	page := max(5, s.frameHeight/2)
	consumed := 0
	for consumed < len(s.inputBuffer) {
		input := s.inputBuffer[consumed:]
		switch input[0] {
		case 3:
			interruptStarlogProcess()
			consumed++
		case 'k':
			s.Scroll(1)
			consumed++
		case 'j':
			s.Scroll(-1)
			consumed++
		case 'g':
			s.scrollOffset.Store(1 << 60)
			consumed++
		case 'G':
			s.FollowLogs()
			consumed++
		case '\x1b':
			used, complete := s.readEscapeInput(input, page)
			if !complete {
				s.inputBuffer = append(s.inputBuffer[:0], input...)
				return
			}
			consumed += used
		default:
			consumed++
		}
	}
	s.inputBuffer = s.inputBuffer[:0]
}

func (s *Starlog) readEscapeInput(input []byte, page int) (int, bool) {
	if len(input) < 2 {
		return 0, false
	}
	if input[1] != '[' {
		return 2, true
	}
	if len(input) < 3 {
		return 0, false
	}
	switch input[2] {
	case 'A':
		s.Scroll(1)
		return 3, true
	case 'B':
		s.Scroll(-1)
		return 3, true
	case 'H':
		s.scrollOffset.Store(1 << 60)
		return 3, true
	case 'F':
		s.FollowLogs()
		return 3, true
	case '5', '6', '1', '4':
		if len(input) < 4 {
			return 0, false
		}
		if input[3] != '~' {
			return 4, true
		}
		switch input[2] {
		case '5':
			s.Scroll(page)
		case '6':
			s.Scroll(-page)
		case '1':
			s.scrollOffset.Store(1 << 60)
		case '4':
			s.FollowLogs()
		}
		return 4, true
	case '<':
		end := 3
		for end < len(input) && input[end] != 'M' && input[end] != 'm' {
			end++
		}
		if end == len(input) {
			return 0, false
		}
		buttonEnd := 3
		for buttonEnd < end && input[buttonEnd] != ';' {
			buttonEnd++
		}
		button, _ := strconv.Atoi(string(input[3:buttonEnd]))
		if button&64 != 0 {
			if button&1 == 0 {
				s.Scroll(3)
			} else {
				s.Scroll(-3)
			}
		}
		return end + 1, true
	default:
		return 3, true
	}
}

func (s *Starlog) render(now time.Time) {
	width, height := 100, 32
	if s.outputFD >= 0 {
		if w, h, err := term.GetSize(s.outputFD); err == nil {
			width, height = w, h
		}
	}
	width = max(width, 54)
	height = max(height, 16)
	stats := s.metrics(now)
	entries := s.Snapshot()

	inner := width - 2
	petWidth := min(44, max(26, inner*2/5))
	mood := s.currentMood(now)
	pet, pictured := s.petCrewFrame(mood, now, petWidth-2)
	playback := s.playbackSnapshot()
	var b strings.Builder
	b.Grow(width * height)
	b.WriteString(s.paint(ansiFrost+ansiBold, "╭"+strings.Repeat("─", inner)+"╮"))
	b.WriteByte('\n')

	status, statusColor := "WAITING", ansiGold
	for _, shard := range stats.Shards {
		if shard.Connected {
			status, statusColor = "ONLINE", ansiGreen
			break
		}
	}
	title := "  " + s.paint(ansiGold+ansiBold, "✦ STARLOG") +
		s.paint(ansiMuted, " / ") + s.paint(ansiBlue+ansiBold, s.name)
	if c := s.client.Load(); c != nil && c.Self() != nil {
		title += s.paint(ansiMuted, " / ") + s.paint(ansiPink+ansiBold, c.Self().Tag())
	}
	right := status + "  " + compactDuration(stats.Uptime) + "  "
	b.WriteString(s.boxLine(inner, s.paint(ansiBold, title), s.paint(statusColor+ansiBold, right)))
	b.WriteByte('\n')

	metricLine := "  " + s.metric("CPU", fmt.Sprintf("%5.1f%%", stats.CPUPercent), ansiGreen) +
		"   " + s.metric("HEAP", fmt.Sprintf("%8s", bytesLabel(stats.HeapBytes)), ansiViolet) +
		"   " + s.metric("GO", fmt.Sprintf("%4d", stats.Goroutines), ansiBlue) +
		"   " + s.metric("LOGS", fmt.Sprintf("%5.1f/s", stats.LogsPerSecond), ansiOrange) +
		"   " + s.metric("TOTAL", fmt.Sprintf("%d", stats.TotalLogs), ansiPink)
	b.WriteString(s.boxLine(inner, metricLine, ""))
	b.WriteByte('\n')
	if playback.Title != "" {
		b.WriteString(s.paint(ansiFrost+ansiBold, "├─ MEDIA "+strings.Repeat("─", max(0, inner-8))+"┤"))
		b.WriteByte('\n')
		for _, line := range s.musicCard(playback, now, inner) {
			b.WriteString(s.boxLine(inner, line, ""))
			b.WriteByte('\n')
		}
	}
	overviewLabel := "├" + padVisible("─ PET DECK ", petWidth, "─") + "┬" +
		padVisible("─ GATEWAY ", inner-petWidth-1, "─") + "┤"
	b.WriteString(s.paint(ansiFrost+ansiBold, overviewLabel))
	b.WriteByte('\n')

	statusRows := max(len(pet), len(stats.Shards)+2)
	statusRows = min(statusRows, max(4, height/3))
	for row := 0; row < statusRows; row++ {
		left := ""
		if row < len(pet) {
			left = pet[row]
			if !pictured {
				left = s.petPaint(left)
			}
		}
		left = padVisible("  "+left, petWidth)
		rightText := ""
		if row == 0 {
			rightText = s.paint(ansiBlue+ansiBold, "SHARD MATRIX") + "  " +
				s.paint(ansiMuted, fmt.Sprintf("%d configured", len(stats.Shards)))
		} else if row == 1 {
			gatewayState, gatewayColor := "standby", ansiGold
			if status == "ONLINE" {
				gatewayState, gatewayColor = "healthy", ansiGreen
			}
			rightText = s.paint(ansiMuted, "CONNECTION") + "  " + s.paint(gatewayColor+ansiBold, gatewayState)
		} else if row-2 < len(stats.Shards) {
			shard := stats.Shards[row-2]
			state := "offline"
			stateColor := ansiRed
			if shard.Connected {
				state, stateColor = "online", ansiGreen
			}
			logs := stats.LogsByShard[shard.ID]
			rightText = s.paint(ansiBlue, fmt.Sprintf("shard %-3d", shard.ID)) + " " +
				s.paint(stateColor+ansiBold, state) + "  " +
				s.paint(ansiMuted, "ping ") + s.paint(ansiGold, fmt.Sprintf("%-8s", latencyLabel(shard.Latency))) + " " +
				s.paint(ansiMuted, "seq ") + s.paint(ansiViolet, fmt.Sprintf("%-8d", shard.Sequence)) + " " +
				s.paint(ansiMuted, "logs ") + s.paint(ansiPink, fmt.Sprintf("%d", logs))
		}
		rightWidth := max(0, inner-petWidth-1)
		line := left + s.paint(ansiFrost+ansiBold, "│") + padVisible(" "+rightText, rightWidth)
		b.WriteString(s.paint(ansiFrost+ansiBold, "│") + line + s.paint(ansiFrost+ansiBold, "│") + "\n")
	}

	extraRows := 0
	if playback.Title != "" {
		extraRows = 3
	}
	logRows := height - statusRows - 6 - extraRows
	if logRows < 1 {
		logRows = 1
	}
	logLines := make([]string, 0, len(entries))
	for _, entry := range entries {
		logLines = append(logLines, wrapVisible(s.formatEntry(entry, true), inner)...)
	}
	maximumOffset := max(0, len(logLines)-logRows)
	offset := min(maximumOffset, max(0, int(s.scrollOffset.Load())))
	s.scrollOffset.Store(int64(offset))
	mode := "FOLLOW"
	if offset > 0 {
		mode = fmt.Sprintf("↑ %d ROWS", offset)
	}
	logPrefix := "─ LOG STREAM "
	logFill := strings.Repeat("─", max(0, inner-visibleWidth(logPrefix)-visibleWidth(mode)-1))
	b.WriteString(s.paint(ansiFrost+ansiBold, "├"+logPrefix) + s.paint(ansiBlue+ansiBold, mode) + s.paint(ansiFrost+ansiBold, " "+logFill+"┤"))
	b.WriteByte('\n')
	end := len(logLines) - offset
	start := max(0, end-logRows)
	visibleLines := logLines[start:end]
	for i := 0; i < logRows-len(visibleLines); i++ {
		b.WriteString(s.paint(ansiFrost+ansiBold, "│") + strings.Repeat(" ", inner) + s.paint(ansiFrost+ansiBold, "│") + "\n")
	}
	for _, line := range visibleLines {
		b.WriteString(s.paint(ansiFrost+ansiBold, "│") + padVisible(line, inner) + s.paint(ansiFrost+ansiBold, "│") + "\n")
	}

	footer := "  ↑↓/wheel scroll  •  PgUp/PgDn page  •  End follow  •  Ctrl+C quit  •  60 FPS  "
	footer = truncateVisible(footer, inner)
	b.WriteString(s.paint(ansiFrost+ansiBold, "╰") + s.paint(ansiMuted, footer) +
		s.paint(ansiFrost+ansiBold, strings.Repeat("─", max(0, inner-visibleWidth(footer)))+"╯"))

	s.drawFrame(strings.Split(b.String(), "\n"), width, height)
}

func (s *Starlog) petCrewFrame(mood StarlogMood, now time.Time, width int) (StarlogFrame, bool) {
	pets := s.pets
	if len(pets) == 0 && s.pet.Name != "" {
		pets = []StarlogPet{s.pet}
	}
	if !s.color || len(pets) == 0 {
		return s.petFrame(mood, now), false
	}
	count := min(len(pets), max(1, width/9))
	gap := 1
	cellWidth := (width - gap*(count-1)) / count
	frames := make([]StarlogFrame, 0, count)
	maxRows := 0
	for index, pet := range pets[:count] {
		// Stagger the rare idle blink so the crew feels alive without dancing.
		frame, ok := renderStarPetPicture(pet, mood, now.Add(time.Duration(index)*1300*time.Millisecond), cellWidth)
		if !ok {
			return s.petFrame(mood, now), false
		}
		frames = append(frames, frame)
		maxRows = max(maxRows, len(frame))
	}
	crew := make(StarlogFrame, 0, maxRows+1)
	for row := 0; row < maxRows; row++ {
		var line strings.Builder
		for index, frame := range frames {
			if index > 0 {
				line.WriteString(strings.Repeat(" ", gap))
			}
			value := ""
			if row < len(frame) {
				value = frame[row]
			}
			line.WriteString(centerVisible(value, cellWidth))
		}
		crew = append(crew, line.String())
	}
	var labels strings.Builder
	for index, pet := range pets[:count] {
		if index > 0 {
			labels.WriteString(strings.Repeat(" ", gap))
		}
		color := ansiPink
		switch index % 3 {
		case 0:
			color = ansiGold
		case 1:
			color = ansiBlue
		case 2:
			color = ansiViolet
		}
		labels.WriteString(centerVisible(s.paint(color+ansiBold, pet.Variant), cellWidth))
	}
	return append(crew, labels.String()), true
}

func (s *Starlog) musicCard(playback StarlogPlayback, now time.Time, width int) []string {
	state, stateColor := "PAUSED", ansiGold
	levels := "▁▁▁▁▁▁▁▁"
	if playback.Playing {
		state, stateColor = "PLAYING", ansiGreen
		meters := []string{"▂▄▆▄▃▇▅▂", "▄▇▃▆▂▅▇▄", "▆▃▇▄▅▂▄▇", "▃▆▄▇▂▆▅▃"}
		levels = meters[int(now.UnixMilli()/80)%len(meters)]
	}
	provider := strings.TrimSpace(playback.Provider)
	if provider == "" {
		provider = "Now playing"
	}
	providerColor := ansiPink
	providerMark := "◆"
	if strings.Contains(strings.ToLower(provider), "spotify") {
		provider, providerColor, providerMark = "SPOTIFY", ansiGreen, "●"
	}
	artist := playback.Artist
	if artist == "" {
		artist = provider
	}
	barWidth := max(10, width-42)
	filled := 0
	if playback.Duration > 0 {
		filled = int(float64(barWidth-1) * min(1, float64(playback.Position)/float64(playback.Duration)))
	}
	bar := strings.Repeat("━", filled) + "●" + strings.Repeat("─", max(0, barWidth-filled-1))
	first := "  " + s.paint(providerColor+ansiBold, providerMark+" "+strings.ToUpper(provider)) + "  " +
		s.paint(ansiBlue+ansiBold, truncateVisible(playback.Title, max(12, width/3))) +
		s.paint(ansiMuted, "  /  ") + s.paint(ansiViolet, truncateVisible(artist, max(8, width/4)))
	second := "  " + s.paint(providerColor, levels) + "  " + s.paint(ansiBlue, bar) + "  " +
		s.paint(ansiWhite, mediaDuration(playback.Position)+" / "+mediaDuration(playback.Duration)) + "  " +
		s.paint(stateColor+ansiBold, state)
	return []string{first, second}
}

func (s *Starlog) drawFrame(frame []string, width, height int) {
	s.frameMu.Lock()
	defer s.frameMu.Unlock()

	full := s.frameWidth != width || s.frameHeight != height || len(s.lastFrame) != len(frame)
	var output strings.Builder
	if full {
		output.WriteString("\x1b[2J\x1b[H")
		output.WriteString(strings.Join(frame, "\n"))
	} else {
		for row, line := range frame {
			if line == s.lastFrame[row] {
				continue
			}
			fmt.Fprintf(&output, "\x1b[%d;1H%s\x1b[K", row+1, line)
		}
	}
	s.lastFrame = append(s.lastFrame[:0], frame...)
	s.frameWidth, s.frameHeight = width, height
	if output.Len() == 0 {
		return
	}
	s.writeMu.Lock()
	_, _ = io.WriteString(s.output, output.String())
	s.writeMu.Unlock()
}

func (s *Starlog) writeStream(entry StarlogEntry) {
	interactive := s.interactive()
	colour := s.color && interactive
	line := s.formatEntryWithColor(entry, colour)
	var frame StarlogFrame
	if interactive && s.streamArt && entry.Mood != StarlogCalm && s.pet.Name != "" {
		frame = s.petFrame(entry.Mood, entry.Time)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if len(frame) == 0 {
		_, _ = fmt.Fprintln(s.output, line)
		return
	}
	middle := len(frame) / 2
	for i, art := range frame {
		if colour {
			art = s.petPaint(art)
		}
		if i == middle {
			_, _ = fmt.Fprintln(s.output, padVisible(art, 14)+"  "+line)
		} else {
			_, _ = fmt.Fprintln(s.output, art)
		}
	}
}

func (s *Starlog) formatEntry(entry StarlogEntry, colour bool) string {
	return s.formatEntryWithColor(entry, colour && s.color)
}

func (s *Starlog) formatEntryWithColor(entry StarlogEntry, colour bool) string {
	level := entry.Level.String()
	if len(level) > 5 {
		level = level[:5]
	}
	level = fmt.Sprintf("%-5s", level)
	if colour {
		level = s.paint(levelColour(entry.Level), level)
	}
	var b strings.Builder
	timestamp := entry.Time.Format("15:04:05")
	if colour {
		timestamp = s.paint(ansiMuted, timestamp)
	}
	b.WriteString(timestamp)
	b.WriteString("  ")
	b.WriteString(level)
	b.WriteString("  ")
	if entry.Shard >= 0 {
		shard := fmt.Sprintf("s%d", entry.Shard)
		if colour {
			shard = s.paint(ansiBlue, shard)
		}
		b.WriteString(shard + "  ")
	}
	message := entry.Message
	if colour {
		messageColor := ansiWhite
		switch {
		case entry.Level >= slog.LevelError:
			messageColor = ansiRed + ansiBold
		case entry.Level >= slog.LevelWarn:
			messageColor = ansiGold
		}
		message = s.paint(messageColor, message)
	}
	b.WriteString(message)
	for _, field := range entry.Fields {
		b.WriteByte(' ')
		if colour {
			b.WriteString(s.paint(ansiViolet, field.Key))
			b.WriteString(s.paint(ansiMuted, "="))
			b.WriteString(s.paint(ansiGold, field.Value))
		} else {
			b.WriteString(field.Key)
			b.WriteByte('=')
			b.WriteString(field.Value)
		}
	}
	return b.String()
}

func (s *Starlog) metric(label, value, valueColor string) string {
	return s.paint(ansiMuted, label+" ") + s.paint(valueColor+ansiBold, value)
}

func (s *Starlog) currentMood(now time.Time) StarlogMood {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if now.Sub(s.lastMoodAt) > 4*time.Second {
		return StarlogCalm
	}
	return s.lastMood
}

func (s *Starlog) petFrame(mood StarlogMood, now time.Time) StarlogFrame {
	var animation []StarlogFrame
	switch mood {
	case StarlogBuild:
		animation = s.pet.Build
	case StarlogWarn:
		animation = s.pet.Warn
	case StarlogError:
		animation = s.pet.Error
	default:
		animation = s.pet.Calm
	}
	if len(animation) == 0 {
		return nil
	}
	delay := s.pet.FrameDelay
	if delay <= 0 {
		delay = 300 * time.Millisecond
	}
	return animation[int(now.UnixNano()/int64(delay))%len(animation)]
}

func (s *Starlog) metrics(now time.Time) StarlogMetrics {
	s.metricsMu.Lock()
	if s.metricsAt.IsZero() || now.Sub(s.metricsAt) >= 250*time.Millisecond {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		s.runtimeHeap = mem.HeapAlloc
		s.runtimeGo = runtime.NumGoroutine()
		s.runtimeTotalCPU, s.runtimeIdleCPU = readStarlogCPU()
		s.metricsAt = now
	}
	heap, goroutines := s.runtimeHeap, s.runtimeGo
	totalCPU, idleCPU := s.runtimeTotalCPU, s.runtimeIdleCPU
	s.metricsMu.Unlock()

	s.mu.Lock()
	if s.cpu.at != 0 && totalCPU > s.cpu.total {
		deltaTotal := totalCPU - s.cpu.total
		deltaUsed := (totalCPU - idleCPU) - (s.cpu.total - s.cpu.idle)
		if deltaUsed < 0 {
			deltaUsed = 0
		}
		s.cpu.percent = min(100, 100*deltaUsed/deltaTotal)
	}
	s.cpu.at, s.cpu.total, s.cpu.idle = float64(now.UnixNano()), totalCPU, idleCPU
	if len(s.rateSamples) == 0 || now.Sub(s.rateSamples[len(s.rateSamples)-1].at) >= 100*time.Millisecond {
		s.rateSamples = append(s.rateSamples, starlogRateSample{at: now, total: s.total})
	} else {
		s.rateSamples[len(s.rateSamples)-1] = starlogRateSample{at: now, total: s.total}
	}
	cutoff := now.Add(-5 * time.Second)
	first := 0
	for first < len(s.rateSamples)-1 && s.rateSamples[first].at.Before(cutoff) {
		first++
	}
	if first > 0 {
		copy(s.rateSamples, s.rateSamples[first:])
		s.rateSamples = s.rateSamples[:len(s.rateSamples)-first]
	}
	rate := 0.0
	if len(s.rateSamples) > 1 {
		oldest := s.rateSamples[0]
		if seconds := now.Sub(oldest.at).Seconds(); seconds > 0 {
			rate = float64(s.total-oldest.total) / seconds
		}
	}
	byShard := make(map[int]uint64, len(s.shardLogs))
	for shard, count := range s.shardLogs {
		byShard[shard] = count
	}
	out := StarlogMetrics{
		Uptime:        now.Sub(s.startedAt),
		CPUPercent:    s.cpu.percent,
		HeapBytes:     heap,
		Goroutines:    goroutines,
		LogsPerSecond: rate,
		TotalLogs:     s.total,
		DebugLogs:     s.levels[0],
		InfoLogs:      s.levels[1],
		WarnLogs:      s.levels[2],
		ErrorLogs:     s.levels[3],
		LogsByShard:   byShard,
	}
	s.mu.Unlock()
	if client := s.client.Load(); client != nil {
		out.Shards = client.ShardStatuses()
	}
	sort.Slice(out.Shards, func(i, j int) bool { return out.Shards[i].ID < out.Shards[j].ID })
	return out
}

func readStarlogCPU() (total, idle float64) {
	samples := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}, {Name: "/cpu/classes/idle:cpu-seconds"}}
	metrics.Read(samples)
	return samples[0].Value.Float64(), samples[1].Value.Float64()
}

func (s *Starlog) boxLine(inner int, left, right string) string {
	space := inner - visibleWidth(left) - visibleWidth(right)
	if space < 1 {
		left = truncateVisible(left, max(0, inner-visibleWidth(right)-1))
		space = max(1, inner-visibleWidth(left)-visibleWidth(right))
	}
	return s.paint(ansiFrost+ansiBold, "│") + left + strings.Repeat(" ", space) + right + s.paint(ansiFrost+ansiBold, "│")
}

func (s *Starlog) paint(code, text string) string {
	if !s.color || text == "" {
		return text
	}
	return code + text + ansiReset
}

func (s *Starlog) petPaint(text string) string {
	if !s.color || text == "" {
		return text
	}
	code := ansiBlue
	if s.pet.Color > 0 {
		code = fmt.Sprintf("\x1b[38;5;%dm", s.pet.Color)
	}
	return code + text + ansiReset
}

func levelColour(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return ansiErrorBadge
	case level >= slog.LevelWarn:
		return ansiWarnBadge
	case level >= slog.LevelInfo:
		return ansiGreen + ansiBold
	default:
		return ansiMuted
	}
}

func visibleWidth(value string) int {
	return utf8.RuneCountInString(stripANSI(value))
}

func truncateVisible(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if visibleWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	var clipped strings.Builder
	visible := 0
	for index := 0; index < len(value) && visible < width-1; {
		if sequence, next, ok := ansiAt(value, index); ok {
			clipped.WriteString(sequence)
			index = next
			continue
		}
		r, size := utf8.DecodeRuneInString(value[index:])
		clipped.WriteRune(r)
		visible++
		index += size
	}
	clipped.WriteString("…")
	clipped.WriteString(ansiReset)
	return clipped.String()
}

func wrapVisible(value string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	if visibleWidth(value) <= width {
		return []string{value}
	}
	lines := make([]string, 0, visibleWidth(value)/width+1)
	var line strings.Builder
	active := ""
	visible := 0
	flush := func() {
		if active != "" {
			line.WriteString(ansiReset)
		}
		lines = append(lines, line.String())
		line.Reset()
		if active != "" {
			line.WriteString(active)
		}
		visible = 0
	}
	for index := 0; index < len(value); {
		if sequence, next, ok := ansiAt(value, index); ok {
			line.WriteString(sequence)
			if strings.HasSuffix(sequence, "m") {
				if sequence == ansiReset || sequence == "\x1b[m" {
					active = ""
				} else {
					active += sequence
				}
			}
			index = next
			continue
		}
		r, size := utf8.DecodeRuneInString(value[index:])
		if r == '\n' {
			flush()
			index += size
			continue
		}
		if visible == width {
			flush()
		}
		line.WriteRune(r)
		visible++
		index += size
	}
	if line.Len() > 0 || len(lines) == 0 {
		if active != "" {
			line.WriteString(ansiReset)
		}
		lines = append(lines, line.String())
	}
	return lines
}

func ansiAt(value string, index int) (string, int, bool) {
	if index+1 >= len(value) || value[index] != '\x1b' || value[index+1] != '[' {
		return "", index, false
	}
	end := index + 2
	for end < len(value) {
		character := value[end]
		end++
		if character >= '@' && character <= '~' {
			return value[index:end], end, true
		}
	}
	return "", index, false
}

func padVisible(value string, width int, fill ...string) string {
	f := " "
	if len(fill) > 0 && fill[0] != "" {
		f = fill[0]
	}
	if visibleWidth(value) > width {
		return truncateVisible(value, width)
	}
	return value + strings.Repeat(f, width-visibleWidth(value))
}

func centerVisible(value string, width int) string {
	if visibleWidth(value) >= width {
		return truncateVisible(value, width)
	}
	left := (width - visibleWidth(value)) / 2
	return strings.Repeat(" ", left) + padVisible(value, width-left)
}

func stripANSI(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); {
		if value[i] == '\x1b' && i+1 < len(value) && value[i+1] == '[' {
			i += 2
			for i < len(value) {
				c := value[i]
				i++
				if c >= '@' && c <= '~' {
					break
				}
			}
			continue
		}
		b.WriteByte(value[i])
		i++
	}
	return b.String()
}

func bytesLabel(value uint64) string {
	const (
		kiB = 1024
		miB = 1024 * kiB
		giB = 1024 * miB
	)
	switch {
	case value >= giB:
		return fmt.Sprintf("%.1f GiB", float64(value)/giB)
	case value >= miB:
		return fmt.Sprintf("%.1f MiB", float64(value)/miB)
	case value >= kiB:
		return fmt.Sprintf("%.1f KiB", float64(value)/kiB)
	default:
		return fmt.Sprintf("%d B", value)
	}
}

func compactDuration(value time.Duration) string {
	value = value.Round(time.Second)
	if value < time.Minute {
		return value.String()
	}
	hours := int(value / time.Hour)
	minutes := int(value/time.Minute) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func mediaDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	total := int(value.Round(time.Second) / time.Second)
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func latencyLabel(value time.Duration) string {
	if value <= 0 {
		return "—"
	}
	return value.Round(time.Millisecond).String()
}
