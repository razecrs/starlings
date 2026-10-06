//go:build windows

package starlings

import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	winrt "github.com/saltosystems/winrt-go"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/media/control"
)

type windowsStarlogMedia struct {
	manager *control.GlobalSystemMediaTransportControlsSessionManager
}

func newStarlogPlatformMedia() (starlogPlatformMedia, error) {
	// RO_INIT_MULTITHREADED = 1. RPC_E_CHANGED_MODE is harmless when the host
	// already initialized this thread with another apartment model.
	if err := ole.RoInitialize(1); err != nil && !strings.Contains(err.Error(), "0x80010106") {
		return nil, err
	}
	operation, err := control.GlobalSystemMediaTransportControlsSessionManagerRequestAsync()
	if err != nil {
		return nil, fmt.Errorf("starlings: request Windows media manager: %w", err)
	}
	result, err := awaitWinRT(operation, control.SignatureGlobalSystemMediaTransportControlsSessionManager)
	if err != nil {
		return nil, fmt.Errorf("starlings: await Windows media manager: %w", err)
	}
	return &windowsStarlogMedia{manager: (*control.GlobalSystemMediaTransportControlsSessionManager)(result)}, nil
}

func (w *windowsStarlogMedia) close() {
	if w.manager != nil {
		w.manager.Release()
	}
}

func (w *windowsStarlogMedia) read() (StarlogPlayback, error) {
	session, err := w.manager.GetCurrentSession()
	if err != nil || session == nil {
		return StarlogPlayback{}, err
	}
	defer session.Release()

	provider, _ := session.GetSourceAppUserModelId()
	propertiesOperation, err := session.TryGetMediaPropertiesAsync()
	if err != nil {
		return StarlogPlayback{}, err
	}
	propertiesResult, err := awaitWinRT(propertiesOperation, control.SignatureGlobalSystemMediaTransportControlsSessionMediaProperties)
	if err != nil {
		return StarlogPlayback{}, err
	}
	properties := (*control.GlobalSystemMediaTransportControlsSessionMediaProperties)(propertiesResult)
	defer properties.Release()
	title, _ := properties.GetTitle()
	artist, _ := properties.GetArtist()

	playback := StarlogPlayback{Provider: mediaProviderName(provider), Title: title, Artist: artist}
	if info, infoErr := session.GetPlaybackInfo(); infoErr == nil && info != nil {
		status, _ := info.GetPlaybackStatus()
		playback.Playing = status == control.GlobalSystemMediaTransportControlsSessionPlaybackStatusPlaying
		info.Release()
	}
	if timeline, timelineErr := session.GetTimelineProperties(); timelineErr == nil && timeline != nil {
		position, _ := timeline.GetPosition()
		start, _ := timeline.GetStartTime()
		end, _ := timeline.GetEndTime()
		playback.Position = time.Duration(position.Duration) * 100 * time.Nanosecond
		playback.Duration = time.Duration(end.Duration-start.Duration) * 100 * time.Nanosecond
		timeline.Release()
	}
	return playback, nil
}

func awaitWinRT(operation *foundation.IAsyncOperation, resultSignature string) (unsafe.Pointer, error) {
	if operation == nil {
		return nil, fmt.Errorf("starlings: nil WinRT async operation")
	}
	defer operation.Release()
	type completion struct {
		result unsafe.Pointer
		err    error
	}
	done := make(chan completion, 1)
	delegateIID := winrt.ParameterizedInstanceGUID(foundation.GUIDAsyncOperationCompletedHandler, resultSignature)
	handler := foundation.NewAsyncOperationCompletedHandler(
		ole.NewGUID(delegateIID),
		func(_ *foundation.AsyncOperationCompletedHandler, async *foundation.IAsyncOperation, status foundation.AsyncStatus) {
			if status != foundation.AsyncStatusCompleted {
				done <- completion{err: fmt.Errorf("starlings: WinRT media operation status %d", status)}
				return
			}
			result, err := async.GetResults()
			if err != nil {
				err = fmt.Errorf("get results: %w", err)
			}
			done <- completion{result: result, err: err}
		},
	)
	defer handler.Release()
	if err := operation.SetCompleted(handler); err != nil {
		return nil, fmt.Errorf("set completion handler: %w", err)
	}
	completed := <-done
	return completed.result, completed.err
}

func mediaProviderName(applicationID string) string {
	lower := strings.ToLower(applicationID)
	switch {
	case strings.Contains(lower, "spotify"):
		return "Spotify"
	case strings.Contains(lower, "chrome"):
		return "Chrome"
	case strings.Contains(lower, "firefox"):
		return "Firefox"
	case strings.Contains(lower, "edge"):
		return "Edge"
	}
	applicationID = strings.TrimSuffix(applicationID, ".exe")
	if cut := strings.LastIndexAny(applicationID, ".!/"); cut >= 0 {
		applicationID = applicationID[cut+1:]
	}
	return applicationID
}
