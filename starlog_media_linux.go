//go:build linux

package starlings

import (
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

type linuxStarlogMedia struct {
	bus *dbus.Conn
}

func newStarlogPlatformMedia() (starlogPlatformMedia, error) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	return &linuxStarlogMedia{bus: bus}, nil
}

func (l *linuxStarlogMedia) close() { _ = l.bus.Close() }

func (l *linuxStarlogMedia) read() (StarlogPlayback, error) {
	var names []string
	if err := l.bus.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return StarlogPlayback{}, err
	}
	var fallback StarlogPlayback
	for _, name := range names {
		if !strings.HasPrefix(name, "org.mpris.MediaPlayer2.") {
			continue
		}
		playback, err := readMPRIS(l.bus.Object(name, "/org/mpris/MediaPlayer2"), name)
		if err != nil || playback.Title == "" {
			continue
		}
		if playback.Playing {
			return playback, nil
		}
		if fallback.Title == "" {
			fallback = playback
		}
	}
	return fallback, nil
}

func readMPRIS(object dbus.BusObject, busName string) (StarlogPlayback, error) {
	status, err := mprisString(object, "PlaybackStatus")
	if err != nil {
		return StarlogPlayback{}, err
	}
	metadataVariant, err := object.GetProperty("org.mpris.MediaPlayer2.Player.Metadata")
	if err != nil {
		return StarlogPlayback{}, err
	}
	metadata, ok := metadataVariant.Value().(map[string]dbus.Variant)
	if !ok {
		return StarlogPlayback{}, fmt.Errorf("starlings: invalid MPRIS metadata")
	}
	provider := strings.TrimPrefix(busName, "org.mpris.MediaPlayer2.")
	if identity, identityErr := mprisInterfaceString(object, "org.mpris.MediaPlayer2.Identity"); identityErr == nil && identity != "" {
		provider = identity
	}
	playback := StarlogPlayback{
		Provider: provider,
		Title:    mprisMetadataString(metadata, "xesam:title"),
		Artist:   strings.Join(mprisMetadataStrings(metadata, "xesam:artist"), ", "),
		Duration: time.Duration(mprisMetadataInt64(metadata, "mpris:length")) * time.Microsecond,
		Playing:  strings.EqualFold(status, "Playing"),
	}
	if position, positionErr := object.GetProperty("org.mpris.MediaPlayer2.Player.Position"); positionErr == nil {
		if value, ok := position.Value().(int64); ok {
			playback.Position = time.Duration(value) * time.Microsecond
		}
	}
	return playback, nil
}

func mprisString(object dbus.BusObject, property string) (string, error) {
	return mprisInterfaceString(object, "org.mpris.MediaPlayer2.Player."+property)
}

func mprisInterfaceString(object dbus.BusObject, property string) (string, error) {
	variant, err := object.GetProperty(property)
	if err != nil {
		return "", err
	}
	value, _ := variant.Value().(string)
	return value, nil
}

func mprisMetadataString(metadata map[string]dbus.Variant, key string) string {
	value, _ := metadata[key].Value().(string)
	return value
}

func mprisMetadataStrings(metadata map[string]dbus.Variant, key string) []string {
	value, _ := metadata[key].Value().([]string)
	return value
}

func mprisMetadataInt64(metadata map[string]dbus.Variant, key string) int64 {
	value, _ := metadata[key].Value().(int64)
	return value
}
