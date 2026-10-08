package voice

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/razecrs/starlings"
)

// FFmpegOpusProvider decodes anything FFmpeg understands into the 20 ms Opus
// frames Discord voice expects. This is useful for common audio files and for
// taking the audio track from a video.
type FFmpegOpusProvider struct {
	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
	stdout io.ReadCloser
	reader *bufio.Reader
	stderr bytes.Buffer

	packets [][]byte
	partial []byte
	done    chan struct{}
	closed  atomic.Bool
	eof     atomic.Bool
	once    sync.Once
	errMu   sync.Mutex
	waitErr error

	title    string
	artist   string
	duration time.Duration
	frames   atomic.Uint64
	playing  atomic.Bool
}

// NewFFmpegOpusProvider starts FFmpeg for path. The context controls the whole
// playback process; cancelling it stops FFmpeg.
func NewFFmpegOpusProvider(ctx context.Context, path string) (*FFmpegOpusProvider, error) {
	if path == "" {
		return nil, errors.New("voice: FFmpeg input path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("voice: FFmpeg input: %w", err)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("voice: ffmpeg is not installed or not on PATH")
	}

	processCtx, cancel := context.WithCancel(ctx)
	title, artist, duration := probeAudioFile(path)
	p := &FFmpegOpusProvider{
		ctx: processCtx, cancel: cancel, done: make(chan struct{}),
		title: title, artist: artist, duration: duration,
	}
	p.cmd = exec.CommandContext(processCtx, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-i", path, "-map", "0:a:0", "-vn",
		"-ac", "2", "-ar", "48000", "-c:a", "libopus",
		"-application", "audio", "-frame_duration", "20",
		"-f", "opus", "pipe:1",
	)
	p.cmd.Stderr = &p.stderr
	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("voice: opening FFmpeg output: %w", err)
	}
	p.reader = bufio.NewReaderSize(p.stdout, 64*1024)
	if err = p.cmd.Start(); err != nil {
		cancel()
		_ = p.stdout.Close()
		return nil, fmt.Errorf("voice: starting FFmpeg: %w", err)
	}
	go func() {
		err := p.cmd.Wait()
		p.playing.Store(false)
		if p.closed.Load() || processCtx.Err() != nil {
			err = nil
		} else if err != nil && p.stderr.Len() > 0 {
			err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(p.stderr.Bytes()))
		}
		p.errMu.Lock()
		p.waitErr = err
		p.errMu.Unlock()
		close(p.done)
	}()
	return p, nil
}

// PlayFile starts an FFmpeg provider and plays it through the connection.
func (v *Connection) PlayFile(ctx context.Context, path string) (*FFmpegOpusProvider, error) {
	provider, err := NewFFmpegOpusProvider(ctx, path)
	if err != nil {
		return nil, err
	}
	if err = v.Play(provider); err != nil {
		provider.Close()
		return nil, err
	}
	return provider, nil
}

// ProvideOpusFrame implements OpusProvider.
func (p *FFmpegOpusProvider) ProvideOpusFrame() ([]byte, error) {
	if p.eof.Load() {
		return nil, io.EOF
	}
	for {
		if len(p.packets) > 0 {
			packet := p.packets[0]
			p.packets = p.packets[1:]
			if bytes.HasPrefix(packet, []byte("OpusHead")) || bytes.HasPrefix(packet, []byte("OpusTags")) || len(packet) == 0 {
				continue
			}
			p.frames.Add(1)
			p.playing.Store(true)
			return packet, nil
		}
		if err := p.readPage(); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || p.closed.Load() || (p.ctx != nil && p.ctx.Err() != nil) {
				p.eof.Store(true)
				p.playing.Store(false)
				return nil, io.EOF
			}
			return nil, err
		}
	}
}

// StarlogPlayback reports progress from Opus frames actually consumed by the
// voice sender, so buffering and network stalls cannot make the UI drift.
func (p *FFmpegOpusProvider) StarlogPlayback() starlings.StarlogPlayback {
	if p == nil {
		return starlings.StarlogPlayback{}
	}
	position := time.Duration(p.frames.Load()) * 20 * time.Millisecond
	if p.duration > 0 && position > p.duration {
		position = p.duration
	}
	return starlings.StarlogPlayback{
		Provider: "Discord voice", Title: p.title, Artist: p.artist, Position: position,
		Duration: p.duration, Playing: p.playing.Load(),
	}
}

func probeAudioFile(path string) (title, artist string, duration time.Duration) {
	title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return title, "", 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, ffprobe,
		"-v", "error", "-show_entries", "format=duration:format_tags=title,artist",
		"-of", "json", path).Output()
	if err != nil {
		return title, "", 0
	}
	var result struct {
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if json.Unmarshal(output, &result) != nil {
		return title, "", 0
	}
	for key, value := range result.Format.Tags {
		switch strings.ToLower(key) {
		case "title":
			if value != "" {
				title = value
			}
		case "artist":
			artist = value
		}
	}
	if seconds, err := strconv.ParseFloat(result.Format.Duration, 64); err == nil && seconds > 0 {
		duration = time.Duration(seconds * float64(time.Second))
	}
	return title, artist, duration
}

func (p *FFmpegOpusProvider) readPage() error {
	header := make([]byte, 27)
	if _, err := io.ReadFull(p.reader, header); err != nil {
		return err
	}
	if !bytes.Equal(header[:4], []byte("OggS")) || header[4] != 0 {
		return errors.New("voice: FFmpeg returned invalid Ogg Opus data")
	}
	lacing := make([]byte, int(header[26]))
	if _, err := io.ReadFull(p.reader, lacing); err != nil {
		return err
	}
	bodyLen := 0
	for _, size := range lacing {
		bodyLen += int(size)
	}
	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(p.reader, body); err != nil {
		return err
	}

	offset := 0
	for _, size := range lacing {
		end := offset + int(size)
		p.partial = append(p.partial, body[offset:end]...)
		offset = end
		if size < 255 {
			packet := append([]byte(nil), p.partial...)
			p.packets = append(p.packets, packet)
			p.partial = p.partial[:0]
		}
	}
	return nil
}

// Wait blocks until FFmpeg exits and reports any decoding error.
func (p *FFmpegOpusProvider) Wait() error {
	<-p.done
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.waitErr
}

// Close stops FFmpeg. It is safe to call more than once.
func (p *FFmpegOpusProvider) Close() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.closed.Store(true)
		p.playing.Store(false)
		p.cancel()
		_ = p.stdout.Close()
		<-p.done
	})
}

var _ OpusProvider = (*FFmpegOpusProvider)(nil)
