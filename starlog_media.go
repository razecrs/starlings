package starlings

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"
)

type starlogMediaSnapshot struct {
	playback StarlogPlayback
	at       time.Time
}

type starlogSystemMedia struct {
	snapshot atomic.Pointer[starlogMediaSnapshot]
}

// StarlogMediaReader reads the operating system's current media session. The
// sysmedia package provides readers for Windows and Linux.
type StarlogMediaReader interface {
	ReadPlayback() (StarlogPlayback, error)
	Close()
}

// StarlogMediaSource shows the operating system's media session in the
// now-playing strip when no attached Discord voice or file source is active.
// open runs when the dashboard starts, on an OS thread reserved for the
// reader, and Starlog polls the reader once a second. Most bots use
// sysmedia.Option instead of calling this directly.
func StarlogMediaSource(open func() (StarlogMediaReader, error)) StarlogOption {
	return func(s *Starlog) { s.mediaOpen = open }
}

func newStarlogSystemMedia(ctx context.Context, open func() (StarlogMediaReader, error)) *starlogSystemMedia {
	media := &starlogSystemMedia{}
	go media.run(ctx, open)
	return media
}

func (s *starlogSystemMedia) run(ctx context.Context, open func() (StarlogMediaReader, error)) {
	// WinRT apartments are thread-affine. Keeping the poller on one OS thread
	// also costs nothing on MPRIS platforms and makes platform adapters simple.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	reader, err := open()
	if err != nil || reader == nil {
		return
	}
	defer reader.Close()
	poll := func() {
		playback, err := reader.ReadPlayback()
		if err != nil || playback.Title == "" {
			s.snapshot.Store(nil)
			return
		}
		s.snapshot.Store(&starlogMediaSnapshot{playback: playback, at: time.Now()})
	}
	poll()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

func (s *starlogSystemMedia) StarlogPlayback() StarlogPlayback {
	if s == nil {
		return StarlogPlayback{}
	}
	snapshot := s.snapshot.Load()
	if snapshot == nil {
		return StarlogPlayback{}
	}
	playback := snapshot.playback
	if playback.Playing {
		playback.Position += time.Since(snapshot.at)
		if playback.Duration > 0 && playback.Position > playback.Duration {
			playback.Position = playback.Duration
		}
	}
	return playback
}
