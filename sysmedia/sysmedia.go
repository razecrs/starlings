// Package sysmedia shows the operating system's current media session in
// Starlog's now-playing strip. Windows uses System Media Transport Controls
// and Linux uses MPRIS; other platforms show nothing.
//
// It is a separate package so bots that do not use it never link the WinRT
// or D-Bus bindings:
//
//	logs := starlings.NewStarlog("my bot", sysmedia.Option())
package sysmedia

import "github.com/razecrs/starlings"

// Option makes Starlog fall back to the system media session when no Discord
// voice or file source is playing.
func Option() starlings.StarlogOption {
	return starlings.StarlogMediaSource(open)
}
