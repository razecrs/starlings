package starlings

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	discordvoice "github.com/disgoorg/disgo/voice"
)

type sliceOpusProvider struct {
	frames [][]byte
	index  int
	closed chan struct{}
	once   sync.Once
}

func (p *sliceOpusProvider) ProvideOpusFrame() ([]byte, error) {
	if p.index == len(p.frames) {
		return nil, io.EOF
	}
	f := p.frames[p.index]
	p.index++
	return f, nil
}

func (p *sliceOpusProvider) Close() { p.once.Do(func() { close(p.closed) }) }

type recordingAudioTransport struct {
	mu       sync.Mutex
	frames   [][]byte
	speaking []discordvoice.SpeakingFlags
	done     chan struct{}
}

func (t *recordingAudioTransport) WriteOpus(frame []byte) error {
	t.mu.Lock()
	t.frames = append(t.frames, append([]byte(nil), frame...))
	t.mu.Unlock()
	return nil
}

func (t *recordingAudioTransport) SetSpeaking(_ context.Context, flags discordvoice.SpeakingFlags) error {
	t.mu.Lock()
	t.speaking = append(t.speaking, flags)
	t.mu.Unlock()
	if flags == discordvoice.SpeakingFlagNone {
		close(t.done)
	}
	return nil
}

func TestBufferedAudioSenderPacesAndFlushes(t *testing.T) {
	provider := &sliceOpusProvider{
		frames: [][]byte{{1}, {2}, {3}, {4}, {5}, {6}},
		closed: make(chan struct{}),
	}
	transport := &recordingAudioTransport{done: make(chan struct{})}
	sender := newBufferedAudioSender(discardLogger(), provider, transport, time.Millisecond, 3, 8)
	sender.Open()

	select {
	case <-transport.done:
	case <-time.After(time.Second):
		t.Fatal("audio sender did not finish")
	}

	transport.mu.Lock()
	defer transport.mu.Unlock()
	if got, want := len(transport.frames), len(provider.frames)+5; got != want {
		t.Fatalf("sent %d frames, want %d", got, want)
	}
	for i, want := range provider.frames {
		if got := transport.frames[i][0]; got != want[0] {
			t.Fatalf("frame %d = %d, want %d", i, got, want[0])
		}
	}
	if len(transport.speaking) != 2 || transport.speaking[0] != discordvoice.SpeakingFlagMicrophone || transport.speaking[1] != discordvoice.SpeakingFlagNone {
		t.Fatalf("speaking transitions = %v", transport.speaking)
	}
	select {
	case <-provider.closed:
	default:
		t.Fatal("provider was not closed")
	}
}

func TestBufferedAudioSenderPlaysShortClip(t *testing.T) {
	provider := &sliceOpusProvider{frames: [][]byte{{1}, {2}}, closed: make(chan struct{})}
	transport := &recordingAudioTransport{done: make(chan struct{})}
	sender := newBufferedAudioSender(discardLogger(), provider, transport, time.Millisecond, 10, 10)
	sender.Open()

	select {
	case <-transport.done:
	case <-time.After(time.Second):
		t.Fatal("short clip did not finish")
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if got, want := len(transport.frames), 7; got != want {
		t.Fatalf("sent %d frames, want %d", got, want)
	}
}
