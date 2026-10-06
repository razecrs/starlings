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

type starlogPlatformMedia interface {
	read() (StarlogPlayback, error)
	close()
}

func newStarlogSystemMedia(ctx context.Context) *starlogSystemMedia {
	media := &starlogSystemMedia{}
	go media.run(ctx)
	return media
}

func (s *starlogSystemMedia) run(ctx context.Context) {
	// WinRT apartments are thread-affine. Keeping the poller on one OS thread
	// also costs nothing on MPRIS platforms and makes platform adapters simple.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	reader, err := newStarlogPlatformMedia()
	if err != nil {
		return
	}
	defer reader.close()
	poll := func() {
		playback, err := reader.read()
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
