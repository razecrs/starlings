//go:build windows

package sysmedia

import (
	"os"
	"testing"
)

func TestWindowsSystemMediaIntegration(t *testing.T) {
	if os.Getenv("STARLINGS_TEST_SYSTEM_MEDIA") == "" {
		t.Skip("set STARLINGS_TEST_SYSTEM_MEDIA=1 to query the desktop media session")
	}
	reader, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	playback, err := reader.ReadPlayback()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("provider=%q title=%q artist=%q playing=%v position=%s duration=%s",
		playback.Provider, playback.Title, playback.Artist, playback.Playing,
		playback.Position, playback.Duration)
}
