package voice

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	discordvoice "github.com/disgoorg/disgo/voice"
)

const (
	voiceFrameInterval = 20 * time.Millisecond
	voicePrebuffer     = 10 // 200 ms: enough to absorb short decoder stalls.
	voiceBufferSize    = 50 // one second, without reading a whole track into RAM.
)

// audioTransport is deliberately tiny so the pacing logic can be tested
// without constructing a Discord voice connection.
type audioTransport interface {
	WriteOpus([]byte) error
	SetSpeaking(context.Context, discordvoice.SpeakingFlags) error
}

type connAudioTransport struct{ conn discordvoice.Conn }

func (t connAudioTransport) WriteOpus(frame []byte) error {
	_, err := t.conn.UDP().Write(frame)
	return err
}

func (t connAudioTransport) SetSpeaking(ctx context.Context, flags discordvoice.SpeakingFlags) error {
	return t.conn.SetSpeaking(ctx, flags)
}

func newAudioSender(logger *slog.Logger, provider discordvoice.OpusFrameProvider, conn discordvoice.Conn) discordvoice.AudioSender {
	return newBufferedAudioSender(logger, provider, connAudioTransport{conn: conn}, voiceFrameInterval, voicePrebuffer, voiceBufferSize)
}

type bufferedAudioSender struct {
	logger    *slog.Logger
	provider  discordvoice.OpusFrameProvider
	transport audioTransport
	interval  time.Duration
	prebuffer int
	frames    chan []byte

	ctx          context.Context
	cancel       context.CancelFunc
	openOnce     sync.Once
	closeOnce    sync.Once
	providerOnce sync.Once
}

func newBufferedAudioSender(logger *slog.Logger, provider discordvoice.OpusFrameProvider, transport audioTransport, interval time.Duration, prebuffer, capacity int) *bufferedAudioSender {
	ctx, cancel := context.WithCancel(context.Background())
	return &bufferedAudioSender{
		logger: logger, provider: provider, transport: transport,
		interval: interval, prebuffer: prebuffer, frames: make(chan []byte, capacity),
		ctx: ctx, cancel: cancel,
	}
}

func (s *bufferedAudioSender) Open() {
	s.openOnce.Do(func() { go s.run() })
}

func (s *bufferedAudioSender) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.closeProvider()
	})
}

func (s *bufferedAudioSender) closeProvider() {
	s.providerOnce.Do(s.provider.Close)
}

func (s *bufferedAudioSender) run() {
	errCh := make(chan error, 1)
	go s.fill(errCh)

	queue, open := s.prime()
	if len(queue) == 0 {
		if open {
			return
		}
		s.logProviderError(errCh)
		return
	}
	if err := s.speaking(discordvoice.SpeakingFlagMicrophone); err != nil {
		s.handleError("starting voice playback", err)
		return
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		var frame []byte
		if len(queue) != 0 {
			frame, queue = queue[0], queue[1:]
		} else if open {
			select {
			case <-s.ctx.Done():
				return
			case frame, open = <-s.frames:
				if !open {
					continue
				}
			}
		} else {
			break
		}

		if err := s.transport.WriteOpus(frame); err != nil {
			s.handleError("sending voice audio", err)
			return
		}
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}

	// Discord recommends five silence packets to flush the decoder cleanly.
	for range 5 {
		if err := s.transport.WriteOpus(discordvoice.SilenceAudioFrame); err != nil {
			s.handleError("finishing voice playback", err)
			return
		}
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
	if err := s.speaking(discordvoice.SpeakingFlagNone); err != nil {
		s.handleError("stopping voice playback", err)
	}
	s.logProviderError(errCh)
}

// prime waits for a useful cushion before playback starts. Short clips begin
// as soon as EOF arrives instead of waiting for an impossible ten frames.
func (s *bufferedAudioSender) prime() (queue [][]byte, open bool) {
	open = true
	for len(queue) < s.prebuffer && open {
		select {
		case <-s.ctx.Done():
			return nil, true
		case frame, ok := <-s.frames:
			open = ok
			if ok {
				queue = append(queue, frame)
			}
		}
	}
	return queue, open
}

func (s *bufferedAudioSender) fill(errCh chan<- error) {
	defer close(s.frames)
	defer s.closeProvider()
	for {
		frame, err := s.provider.ProvideOpusFrame()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				errCh <- err
			}
			return
		}
		if len(frame) == 0 {
			continue
		}
		frame = append([]byte(nil), frame...)
		select {
		case <-s.ctx.Done():
			return
		case s.frames <- frame:
		}
	}
}

func (s *bufferedAudioSender) speaking(flags discordvoice.SpeakingFlags) error {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	return s.transport.SetSpeaking(ctx, flags)
}

func (s *bufferedAudioSender) handleError(message string, err error) {
	if errors.Is(err, net.ErrClosed) || errors.Is(err, discordvoice.ErrGatewayNotConnected) || s.ctx.Err() != nil {
		return
	}
	s.logger.Error("voice: "+message, "err", err)
}

func (s *bufferedAudioSender) logProviderError(errCh <-chan error) {
	select {
	case err := <-errCh:
		if err != nil && s.ctx.Err() == nil {
			s.logger.Error("voice: reading voice audio", "err", err)
		}
	default:
	}
}

var _ discordvoice.AudioSender = (*bufferedAudioSender)(nil)
